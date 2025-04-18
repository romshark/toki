package codeparse

import (
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	tik "github.com/romshark/tik/tik-go"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/log"
	"golang.org/x/text/language"
	"golang.org/x/tools/go/packages"
)

const (
	targetPackage = "github.com/romshark/toki"
	targetType    = targetPackage + ".Reader"
	typeGender    = "Gender"

	FuncTypeText = "Text"
)

type Statistics struct {
	TextTotal      atomic.Int64
	NewTexts       atomic.Int64
	FilesTraversed atomic.Int64
}

type Parser struct {
	tikParser     *tik.Parser
	arbDecoder    *arb.Decoder
	icuTranslator *tik.ICUTranslator
}

var defaultTIKConfig = tik.DefaultConfig()

func NewParser() *Parser {
	return &Parser{
		tikParser:     tik.NewParser(defaultTIKConfig),
		arbDecoder:    arb.NewDecoder(),
		icuTranslator: tik.NewICUTranslator(defaultTIKConfig),
	}
}

type FnOnSrcErr func(pos token.Position, err error)

type Text struct {
	Position token.Position
	TIK      tik.TIK
	Comments []string
}

func (t Text) Context() string {
	f := t.TIK.Tokens[0]
	if f.Type != tik.TokenTypeContext {
		return ""
	}
	return f.String(t.TIK.Raw)
}

type SourceError struct {
	token.Position
	Err error
}

type Scan struct {
	Texts        []Text
	SourceErrors []SourceError
	ARBFiles     []*arb.File
}

func (p *Parser) Parse(
	pathPattern, bundlePkg string,
	locale language.Tag,
	trimpath bool,
) (stats *Statistics, scan *Scan, err error) {
	fset := token.NewFileSet()
	stats = new(Statistics)

	cfg := &packages.Config{
		Mode: packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedDeps |
			packages.NeedName |
			packages.NeedModule,
		Fset: fset,
	}
	pkgs, err := packages.Load(cfg, pathPattern+"/...")
	if err != nil {
		return stats, nil, fmt.Errorf("loading packages: %w", err)
	}

	scan = new(Scan)

	pkgBundle := findBundlePkg(bundlePkg, pkgs)
	if pkgBundle != nil {
		log.Verbosef("bundle detected: %s\n", pkgBundle.Dir)
		err = p.collectARBFiles(pkgBundle.Dir, scan)
		if err != nil {
			return stats, scan, fmt.Errorf("searching .arb files: %w", err)
		}
	}

	p.collectTexts(fset, pkgs, bundlePkg, pathPattern, trimpath, stats, scan)

	// TODO: process bundle package
	_ = pkgBundle

	return stats, scan, nil
}

func findBundlePkg(bundlePkg string, pkgs []*packages.Package) *packages.Package {
	for _, pkg := range pkgs {
		if isPkgBundle(bundlePkg, pkg) {
			return pkg
		}
	}
	return nil
}

func (p *Parser) collectARBFiles(bundlePkgDir string, scan *Scan) error {
	return forFileInDir(bundlePkgDir, func(fileName string) error {
		if filepath.Ext(fileName) != ".arb" {
			return nil
		}
		log.Verbosef("translation file detected: %s\n", fileName)

		f, err := os.OpenFile(fileName, os.O_RDONLY, 0o644)
		if err != nil {
			return err
		}

		arbFile, err := p.arbDecoder.Decode(f)
		if err != nil {
			return fmt.Errorf("parsing .arb file: %w", err)
		}

		scan.ARBFiles = append(scan.ARBFiles, arbFile)

		return nil
	})
}

func (p *Parser) collectTexts(
	fset *token.FileSet, pkgs []*packages.Package,
	bundlePkg, pathPattern string, trimpath bool,
	stats *Statistics, scan *Scan,
) {
	for _, pkg := range pkgs {
		if isPkgBundle(bundlePkg, pkg) {
			continue
		}
		for _, file := range pkg.Syntax {
			stats.FilesTraversed.Add(1)
			for _, decl := range file.Decls {
				ast.Inspect(decl, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}

					selector, ok := call.Fun.(*ast.SelectorExpr)
					if !ok { // Not a function selector.
						return true
					}

					obj := pkg.TypesInfo.Uses[selector.Sel]
					if obj == nil { // Not the right package and type.
						return true
					}

					methodType, ok := obj.Type().(*types.Signature)
					if !ok {
						return true
					}

					recv := methodType.Recv()
					if recv == nil || recv.Type().String() != targetType {
						return true // Not the right receiver type.
					}

					if obj.Pkg() == nil || obj.Pkg().Path() != targetPackage {
						return true // Not from the target package.
					}

					funcType := selector.Sel.Name
					switch funcType {
					case FuncTypeText:
						stats.TextTotal.Add(1)
					default:
						return true // Not the right methods.
					}

					posCall := fset.Position(call.Pos())
					if trimpath {
						posCall.Filename = mustTrimPath(pathPattern, posCall.Filename)
					}

					tikVal, ok := p.parseTIK(fset, pkg, call,
						func(pos token.Position, err error) {
							scan.SourceErrors = append(scan.SourceErrors, SourceError{
								Position: pos, Err: fmt.Errorf("TIK: %w", err),
							})
						})
					if !ok {
						return false
					}

					comments := findLeadingComments(fset, file, call)

					scan.Texts = append(scan.Texts, Text{
						Position: posCall,
						TIK:      tikVal,
						Comments: comments,
					})

					return true
				})
			}
		}
	}
}

func (p *Parser) parseTIK(
	fileset *token.FileSet, pkg *packages.Package,
	call *ast.CallExpr, onSrcErr FnOnSrcErr,
) (tk tik.TIK, ok bool) {
	arg := call.Args[0]
	pos := fileset.Position(arg.Pos())
	tv, ok := pkg.TypesInfo.Types[arg]
	if !ok {
		onSrcErr(pos, errors.New("no type info"))
		return tk, false
	}

	if tv.Value == nil {
		onSrcErr(pos, errors.New("not a constant"))
		return tk, false
	}

	if tv.Type.String() != "string" {
		onSrcErr(pos, errors.New("not a string constant"))
		return tk, false
	}

	tikStr := constant.StringVal(tv.Value)

	tk, err := p.tikParser.Parse(tikStr)
	if err != nil {
		onSrcErr(pos, err)
		return tk, false
	}

	var placeholders []tik.Token
	{
		count := 0
		for range tk.Placeholders() {
			count++
		}
		placeholders = make([]tik.Token, count)
	}
	for i, p := range tk.Placeholders() {
		placeholders[i] = p
	}

	ok = true
	index := 0
	for arg := range iterArgs(call) {
		idx := index
		index++
		if idx >= len(placeholders) {
			onSrcErr(pos, fmt.Errorf("arg %v doesn't match any TIK placeholder", arg))
			ok = false
			continue
		}

		placeholder := placeholders[idx]
		switch placeholder.Type {
		case tik.TokenTypeStringPlaceholder:
			if typName, isStr := isString(pkg, arg); !isStr {
				onSrcErr(pos, fmt.Errorf("arg %d must be a string but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeNumber,
			tik.TokenTypeCardinalPluralStart,
			tik.TokenTypeOrdinalPlural:
			if typName, isNum := isNumeric(pkg, arg); !isNum {
				onSrcErr(pos, fmt.Errorf("arg %d must be numeric but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeGenderPronoun:
			if typName, isGender := isTokiGender(pkg, arg); !isGender {
				onSrcErr(pos, fmt.Errorf("arg %d must be toki.Gender but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeTimeShort,
			tik.TokenTypeTimeShortSeconds,
			tik.TokenTypeTimeFullMonthAndDay,
			tik.TokenTypeTimeShortMonthAndDay,
			tik.TokenTypeTimeFullMonthAndYear,
			tik.TokenTypeTimeWeekday,
			tik.TokenTypeTimeDateAndShort,
			tik.TokenTypeTimeYear,
			tik.TokenTypeTimeFull:
			if typName, isTime := isTime(pkg, arg); !isTime {
				onSrcErr(pos, fmt.Errorf("arg %d must be time.Time but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeCurrencyRounded,
			tik.TokenTypeCurrencyFull,
			tik.TokenTypeCurrencyCodeRounded,
			tik.TokenTypeCurrencyCodeFull:
			if typName, isCurrency := isCurrencyAmount(pkg, arg); !isCurrency {
				onSrcErr(pos, fmt.Errorf(
					"arg %d must be currency.Amount but received: %s",
					idx, typName))
				ok = false
				continue
			}
		}
	}

	offset := index
	if d := len(placeholders) - offset; d > 0 {
		for i, placeholder := range placeholders[offset:] {
			onSrcErr(pos, fmt.Errorf("missing argument %d for placeholder (%s)",
				offset+i, placeholder.Type.String()))
			ok = false
		}
	}

	return tk, ok
}

func iterArgs(call *ast.CallExpr) iter.Seq[ast.Expr] {
	if len(call.Args) < 2 {
		return func(yield func(ast.Expr) bool) {}
	}
	secondArg := call.Args[1]
	isEllipsis := call.Ellipsis.IsValid() // true if passed as slice...
	if isEllipsis {
		compositeLit, ok := secondArg.(*ast.CompositeLit)
		if !ok {
			panic("not a composite literal, can't unpack")
		}
		return func(yield func(ast.Expr) bool) {
			for _, e := range compositeLit.Elts {
				// each elt is an ast.Expr, could be an identifier, call, etc.
				if !yield(e) {
					break
				}
			}
		}
	} else {
		return func(yield func(ast.Expr) bool) {
			// Direct variadic args: foo.Text("msg", a, b, c)
			// call.Args[1:] are the variadic arguments
			for _, a := range call.Args[1:] {
				if !yield(a) {
					break
				}
			}
		}
	}
}

func isString(pkg *packages.Package, expr ast.Expr) (actualTypeName string, ok bool) {
	tv, found := pkg.TypesInfo.Types[expr]
	if !found || tv.Type == nil {
		return tv.Type.String(), false
	}

	if b, ok := tv.Type.Underlying().(*types.Basic); ok && b.Kind() == types.String {
		return "string", true
	}

	return tv.Type.String(), false
}

func isNumeric(pkg *packages.Package, expr ast.Expr) (actualTypeName string, ok bool) {
	tv, ok := pkg.TypesInfo.Types[expr]
	if !ok || tv.Type == nil {
		return tv.Type.String(), false
	}

	switch t := tv.Type.Underlying().(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr,
			types.Float32, types.Float64:
			return tv.Type.String(), true
		}
	}
	return tv.Type.String(), false
}

func isTime(pkg *packages.Package, expr ast.Expr) (actualTypeName string, ok bool) {
	tv, found := pkg.TypesInfo.Types[expr]
	if !found || tv.Type == nil {
		return tv.Type.String(), false
	}

	named, ok := tv.Type.(*types.Named)
	if !ok {
		return tv.Type.String(), false
	}

	obj := named.Obj()
	if obj.Pkg() == nil {
		return tv.Type.String(), false
	}

	if obj.Pkg().Path() == "time" && obj.Name() == "Time" {
		return obj.Pkg().Name() + "." + obj.Name(), true // "time.Time"
	}

	// It's a named type, but not time.Time
	return obj.Pkg().Name() + "." + obj.Name(), false
}

func isTokiGender(
	pkg *packages.Package, expr ast.Expr,
) (actualTypeName string, ok bool) {
	tv, found := pkg.TypesInfo.Types[expr]
	if !found || tv.Type == nil {
		return tv.Type.String(), false
	}

	named, ok := tv.Type.(*types.Named)
	if !ok {
		return tv.Type.String(), false
	}

	obj := named.Obj()
	objPkg := obj.Pkg()
	if objPkg == nil {
		return tv.Type.String(), false
	}

	if obj.Name() == typeGender && objPkg.Path() == targetPackage {
		return objPkg.Name() + "." + obj.Name(), true // "toki.Gender"
	}

	// Named type but not the one we want
	return objPkg.Name() + "." + obj.Name(), false
}

func isCurrencyAmount(
	pkg *packages.Package, expr ast.Expr,
) (actualTypeName string, ok bool) {
	tv, found := pkg.TypesInfo.Types[expr]
	if !found || tv.Type == nil {
		return tv.Type.String(), false
	}

	named, ok := tv.Type.(*types.Named)
	if !ok {
		return tv.Type.String(), false
	}

	obj := named.Obj()
	objPkg := obj.Pkg()
	if objPkg == nil {
		return tv.Type.String(), false
	}

	if obj.Name() == "Amount" && objPkg.Path() == "golang.org/x/text/currency" {
		return objPkg.Name() + "." + obj.Name(), true // "currency.Amount"
	}

	return objPkg.Name() + "." + obj.Name(), false
}

func isPkgBundle(bundlePkg string, pkg *packages.Package) bool {
	if c, ok := strings.CutPrefix(pkg.Dir, pkg.Module.Dir); ok {
		if len(c) > 1 && c[0] == '/' && strings.HasSuffix(c[1:], bundlePkg) {
			return true
		}
	}
	return false
}

func mustTrimPath(basePattern, s string) string {
	basePattern = strings.TrimSuffix(basePattern, "/...")
	abs, err := filepath.Abs(basePattern)
	if err != nil {
		panic(fmt.Errorf("getting absolute path: %w", err))
	}
	return strings.TrimPrefix(s, abs)
}

func findLeadingComments(fset *token.FileSet, file *ast.File, call ast.Expr) []string {
	pos := call.Pos()
	var lines []string
	isImmediatelyAbove := func(commentEnd token.Pos) bool {
		end := fset.Position(commentEnd)
		start := fset.Position(pos)
		return start.Line == end.Line+1
	}
	for _, cg := range file.Comments {
		if cg.End() >= pos {
			continue // Only care about comments strictly before.
		}
		if isImmediatelyAbove(cg.End()) {
			for _, c := range cg.List {
				s := strings.TrimPrefix(c.Text, "//")
				s = strings.TrimSpace(s)
				lines = append(lines, s)
			}
		}
	}
	return lines
}

func forFileInDir(dir string, fn func(fileName string) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			if err := fn(entry.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}
