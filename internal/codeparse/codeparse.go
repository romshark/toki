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

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	tik "github.com/romshark/tik/tik-go"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/log"
	"github.com/romshark/toki/internal/sync"
	"golang.org/x/text/language"
	"golang.org/x/tools/go/packages"
)

const (
	FuncTypeString = "String"
	FuncTypeWrite  = "Write"
)

var ErrUnsupportedSelectOption = errors.New("unsupported select option")

type Statistics struct {
	StringCalls    atomic.Int64
	WriteCalls     atomic.Int64
	FilesTraversed atomic.Int64
}

type Parser struct {
	hasher        *xxhash.Digest
	tikParser     *tik.Parser
	arbDecoder    *arb.Decoder
	icuDecoder    *icumsg.Tokenizer
	icuTranslator *tik.ICUTranslator

	genderType string
	readerType string
}

func NewParser(
	hasher *xxhash.Digest,
	tikParser *tik.Parser,
	translatorICU *tik.ICUTranslator,
) *Parser {
	return &Parser{
		hasher:        hasher,
		tikParser:     tikParser,
		arbDecoder:    arb.NewDecoder(),
		icuDecoder:    new(icumsg.Tokenizer),
		icuTranslator: translatorICU,
	}
}

type FnOnSrcErr func(pos token.Position, err error)

type Text struct {
	Position token.Position
	TIK      tik.TIK
	IDHash   string
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

type CatalogStatistics struct {
	MessagesIncomplete atomic.Int64
}

type Catalog struct {
	CatalogStatistics
	ARB         *arb.File
	ARBFileName string
}

type Scan struct {
	Statistics
	TokiVersion   string
	DefaultLocale language.Tag
	Texts         *sync.Slice[Text]
	TextIndexByID *sync.Map[string, int]
	SourceErrors  *sync.Slice[SourceError]
	Catalogs      *sync.Slice[*Catalog]
}

func (p *Parser) Parse(
	pathPattern, bundlePkg string,
	locale language.Tag,
	trimpath bool,
) (scan *Scan, err error) {
	fset := token.NewFileSet()

	conf := &packages.Config{
		Mode: packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedCompiledGoFiles |
			packages.NeedDeps |
			packages.NeedName |
			packages.NeedModule,
		Fset: fset,
	}
	conf.Tests = true
	pkgs, err := packages.Load(conf, pathPattern+"/...")
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	scan = &Scan{
		Texts:         sync.NewSlice[Text](0),
		TextIndexByID: sync.NewMap[string, int](0),
		SourceErrors:  sync.NewSlice[SourceError](0),
		Catalogs:      sync.NewSlice[*Catalog](1),
	}

	pkgBundle := findBundlePkg(bundlePkg, pkgs)
	if pkgBundle != nil {
		log.Verbosef("bundle detected: %s\n", pkgBundle.Dir)
		scan.TokiVersion = getConstantValue(pkgBundle, "TokiVersion")
		defaultLocaleString := getConstantValue(pkgBundle, "DefaultLocale")
		defaultLocale, err := language.Parse(defaultLocaleString)
		if err != nil {
			return nil, fmt.Errorf("invalid DefaultLocale value: %w", err)
		}
		scan.DefaultLocale = defaultLocale

		err = p.collectARBFiles(pkgBundle.Dir, scan)
		if err != nil {
			return scan, fmt.Errorf("searching .arb files: %w", err)
		}
		p.genderType = pkgBundle.PkgPath + ".Gender"
		p.readerType = pkgBundle.PkgPath + ".Reader"
	}

	p.collectTexts(fset, pkgs, bundlePkg, pathPattern, trimpath, scan)

	return scan, nil
}

func findBundlePkg(bundlePkg string, pkgs []*packages.Package) *packages.Package {
	for _, pkg := range pkgs {
		if isPkgBundle(bundlePkg, pkg) {
			return pkg
		}
	}
	return nil
}

// getConstantValue returns the literal value of the constant name declared in package p.
// Empty string is returned if it isn't found, isn't a constant, or has no value.
func getConstantValue(p *packages.Package, name string) string {
	if p == nil || p.Types == nil {
		return ""
	}
	obj := p.Types.Scope().Lookup(name)
	c, ok := obj.(*types.Const)
	if !ok {
		return ""
	}
	val := c.Val()
	if val == nil {
		return ""
	}
	if val.Kind() == constant.String {
		return constant.StringVal(val)
	}
	return val.String()
}

var selectOptionsGender = []string{"male", "female"}

func selectOptions(argName string) (
	[]string, icumsg.OptionsPresencePolicy, icumsg.OptionUnknownPolicy,
) {
	if strings.HasSuffix(argName, "_gender") {
		return selectOptionsGender,
			icumsg.OptionsPresencePolicyRequired,
			icumsg.OptionUnknownPolicyReject
	}
	return nil, 0, 0
}

func (p *Parser) collectARBFiles(bundlePkgDir string, scan *Scan) error {
	return forFileInDir(bundlePkgDir, func(fileName string) error {
		if filepath.Ext(fileName) != ".arb" {
			return nil
		}

		withoutExt := strings.TrimSuffix(fileName, ".arb")
		withoutPrefix, ok := strings.CutPrefix(withoutExt, "catalog.")
		if !ok {
			log.Verbosef("ignoring inactive translation file: %s\n", fileName)
			return nil
		}
		locale, err := language.Parse(withoutPrefix)
		if err != nil {
			log.Verbosef("ignoring inactive translation file: %s\n", fileName)
			return nil
		}

		log.Verbosef("translation file detected (%s): %s\n", locale.String(), fileName)

		path := filepath.Join(bundlePkgDir, fileName)
		f, err := os.OpenFile(path, os.O_RDONLY, 0o644)
		if err != nil {
			return err
		}

		arbFile, err := p.arbDecoder.Decode(f)
		if err != nil {
			return fmt.Errorf("parsing .arb file: %w", err)
		}

		if arbFile.Locale != locale {
			return fmt.Errorf("locale in ARB file (%s) differs from file name (%s): %s",
				arbFile.Locale.String(), locale.String(), fileName)
		}

		catalog := &Catalog{ARB: arbFile, ARBFileName: fileName}

		for _, msg := range arbFile.Messages {
			incomplete := IsMsgIncomplete(scan, arbFile, fileName, &msg)
			if incomplete {
				catalog.MessagesIncomplete.Add(1)
			}
		}

		scan.Catalogs.Append(catalog)

		return nil
	})
}

func IsMsgIncomplete(
	scan *Scan, arbFile *arb.File, fileName string, msg *arb.Message,
) bool {
	incomplete := false
	_ = icumsg.Completeness(
		msg.ICUMessage, msg.ICUMessageTokens, arbFile.Locale,
		selectOptions,
		func(index int) { incomplete = true }, // On incomplete.
		func(index int) { // On rejected.
			name := msg.ICUMessageTokens[index+1].String(
				msg.ICUMessage, msg.ICUMessageTokens,
			)
			scan.SourceErrors.Append(SourceError{
				Err: fmt.Errorf("%w: %q", ErrUnsupportedSelectOption, name),
				Position: token.Position{
					Filename: fileName,
				},
			})
		},
	)
	return incomplete
}

func (p *Parser) collectTexts(
	fset *token.FileSet, pkgs []*packages.Package,
	bundlePkg, pathPattern string, trimpath bool,
	scan *Scan,
) {
	seenFiles := make(map[string]struct{})
	for _, pkg := range pkgs {
		if isPkgBundle(bundlePkg, pkg) {
			continue
		}
		for iFile, file := range pkg.Syntax {
			filePath := pkg.CompiledGoFiles[iFile]
			if _, ok := seenFiles[filePath]; ok {
				continue
			}
			seenFiles[filePath] = struct{}{}

			scan.FilesTraversed.Add(1)
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
					if recv == nil || recv.Type().String() != p.readerType {
						return true // Not the right receiver type.
					}

					funcType := selector.Sel.Name
					posCall := fset.Position(call.Pos())
					argumentOffset := 0
					var tikVal tik.TIK
					switch funcType {
					case FuncTypeString:
						scan.StringCalls.Add(1)
					case FuncTypeWrite:
						argumentOffset = 1
						scan.WriteCalls.Add(1)
					default:
						return true // Not the right methods.
					}

					tikVal, ok = p.parseTIK(fset, pkg, call, argumentOffset,
						func(pos token.Position, err error) {
							scan.SourceErrors.Append(SourceError{
								Position: pos, Err: fmt.Errorf("TIK: %w", err),
							})
						})
					if !ok {
						return false
					}

					if trimpath {
						posCall.Filename = mustTrimPath(pathPattern, posCall.Filename)
					}

					comments := findLeadingComments(fset, file, call)

					id := HashMessage(p.hasher, tikVal.Raw)
					log.Verbosef("%s at %s:%d:%d\n",
						funcType, posCall.Filename, posCall.Line, posCall.Column)
					_ = scan.TextIndexByID.Access(func(s map[string]int) error {
						index := scan.Texts.Append(Text{
							Position: posCall,
							IDHash:   id,
							TIK:      tikVal,
							Comments: comments,
						})
						s[id] = index
						return nil
					})

					return true
				})
			}
			log.Verbosef("traversed file %s\n", filePath)
		}
	}
}

func (p *Parser) parseTIK(
	fileset *token.FileSet, pkg *packages.Package, call *ast.CallExpr,
	methodArgumentOffset int, onSrcErr FnOnSrcErr,
) (tk tik.TIK, ok bool) {
	arg := call.Args[methodArgumentOffset]
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
	for arg := range iterArgs(call, methodArgumentOffset) {
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
			if typName, isGender := p.isTokiGender(pkg, arg); !isGender {
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

func iterArgs(call *ast.CallExpr, argOffset int) iter.Seq[ast.Expr] {
	if len(call.Args)+argOffset < 2 {
		return func(yield func(ast.Expr) bool) {}
	}
	isEllipsis := call.Ellipsis.IsValid() // true if passed as slice...
	if isEllipsis && argOffset+1 < len(call.Args) {
		secondArg := call.Args[argOffset+1]
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
			// Iterate over variadic arguments like: foo.String("msg", a, b, c)
			for _, a := range call.Args[argOffset+1:] {
				if !yield(a) {
					break
				}
			}
		}
	}
}

func HashMessage(hash *xxhash.Digest, tik string) string {
	hash.Reset()
	_, _ = hash.WriteString(tik)
	return fmt.Sprintf("msg%x", hash.Sum64())
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

func (p *Parser) isTokiGender(
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

	if objPkg.Path() == p.genderType {
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
