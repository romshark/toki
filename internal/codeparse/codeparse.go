package codeparse

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/log"
	"github.com/romshark/toki/internal/sync"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	tik "github.com/romshark/tik/tik-go"
	"golang.org/x/text/language"
	"golang.org/x/tools/go/packages"
)

const (
	FuncTypeString = "String"
	FuncTypeWrite  = "Write"
)

var (
	ErrUnsupportedSelectOption    = errors.New("unsupported select option")
	ErrCantUnpackCompositeLiteral = errors.New("can't unpack composite literal")
)

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
	ARBFilePath string
}

func NewScan(defaultLocale language.Tag, tokiVersion string) *Scan {
	return &Scan{
		DefaultLocale: defaultLocale,
		TokiVersion:   tokiVersion,
		Texts:         sync.NewSlice[Text](0),
		TextIndexByID: sync.NewMap[string, int](0),
		SourceErrors:  sync.NewSlice[SourceError](0),
		Catalogs:      sync.NewSlice[*Catalog](0),
	}
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
	env []string, pathPattern, bundlePkgPath string, trimpath bool,
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
		Fset:  fset,
		Tests: true,
		Env:   env,
	}
	pkgs, err := packages.Load(conf, pathPattern+"/...")
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("errors in package %q: %v", pkg.Name, pkg.Errors)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	scan = &Scan{
		Texts:         sync.NewSlice[Text](0),
		TextIndexByID: sync.NewMap[string, int](0),
		SourceErrors:  sync.NewSlice[SourceError](0),
		Catalogs:      sync.NewSlice[*Catalog](1),
	}

	bundlePkg := filepath.Base(bundlePkgPath)
	pkgBundle := findBundlePkg(bundlePkg, pkgs)
	if pkgBundle != nil {
		log.Verbose("bundle detected", slog.String("directory", pkgBundle.Dir))
		scan.TokiVersion = getConstantValue(pkgBundle, "TokiVersion")
		defaultLocaleString := getConstantValue(pkgBundle, "DefaultLocale")
		defaultLocale, err := language.Parse(defaultLocaleString)
		if err != nil {
			return nil, fmt.Errorf("invalid DefaultLocale value: %w", err)
		}
		scan.DefaultLocale = defaultLocale

		err = p.CollectARBFiles(pkgBundle.Dir, scan)
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

func ICUSelectOptions(argName string) (
	[]string, icumsg.OptionsPresencePolicy, icumsg.OptionUnknownPolicy,
) {
	if strings.HasSuffix(argName, "_gender") {
		return selectOptionsGender,
			icumsg.OptionsPresencePolicyRequired,
			icumsg.OptionUnknownPolicyReject
	}
	return nil, 0, 0
}

func (p *Parser) CollectARBFiles(bundlePkgDir string, scan *Scan) error {
	return forFileInDir(bundlePkgDir, func(fileName string) error {
		if filepath.Ext(fileName) != ".arb" {
			return nil
		}

		withoutExt := strings.TrimSuffix(fileName, ".arb")
		withoutPrefix, ok := strings.CutPrefix(withoutExt, "catalog_")
		locale, err := language.Parse(withoutPrefix)
		if !ok || err != nil {
			log.Verbose("ignoring inactive translation file",
				slog.String("file", fileName))
			return nil
		}

		log.Verbose("translation file detected",
			slog.String("locale", locale.String()),
			slog.String("file", fileName))

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

		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("determining absolute file path: %w", err)
		}

		catalog := &Catalog{ARB: arbFile, ARBFilePath: absPath}

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
	_, _ = icumsg.Analyze(
		arbFile.Locale, msg.ICUMessage, msg.ICUMessageTokens,
		ICUSelectOptions,
		func(index int) error { incomplete = true; return nil }, // On incomplete.
		func(indexArgument, indexOption int) error { // On rejected.
			name := msg.ICUMessageTokens[indexArgument+1].String(
				msg.ICUMessage, msg.ICUMessageTokens,
			)
			scan.SourceErrors.Append(SourceError{
				Err: fmt.Errorf("%w: %q", ErrUnsupportedSelectOption, name),
				Position: token.Position{
					Filename: fileName,
				},
			})
			return nil
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
					log.Verbose(funcType,
						slog.String("pos", log.FmtPos(posCall)))
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
			log.Verbose("traversed file", slog.String("file", filePath))
		}
	}
}

func mustFmtExpr(e ast.Expr) string {
	var b bytes.Buffer
	err := format.Node(&b, token.NewFileSet(), e)
	if err != nil {
		panic(err)
	}
	return b.String()
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
	seq, err := iterArgs(call, methodArgumentOffset)
	if err != nil {
		onSrcErr(pos, err)
		return tk, false
	}
	for arg := range seq {
		idx := index
		index++
		if idx >= len(placeholders) {
			onSrcErr(pos, fmt.Errorf(
				"arg %s doesn't match any TIK placeholder",
				mustFmtExpr(arg),
			))
			ok = false
			continue
		}

		placeholder := placeholders[idx]
		switch placeholder.Type {
		case tik.TokenTypeTextWithGender:
			if typName, isStr := p.isStringValue(pkg, arg); !isStr {
				onSrcErr(pos, fmt.Errorf(
					"arg %d must be a String with gender but received: %s",
					idx, typName))
				ok = false
				continue
			}
		case tik.TokenTypeText:
			if typName, isStr := isString(pkg, arg); !isStr {
				onSrcErr(pos, fmt.Errorf("arg %d must be a string but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeInteger:
			if typName, isInt := isInteger(pkg, arg); !isInt {
				onSrcErr(pos, fmt.Errorf("arg %d must be an integer but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeNumber:
			if typName, isFloat := isFloat(pkg, arg); !isFloat {
				onSrcErr(pos, fmt.Errorf("arg %d must be a float but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeCardinalPluralStart,
			tik.TokenTypeOrdinalPlural:
			if typName, isNum := isNumeric(pkg, arg); !isNum {
				onSrcErr(pos, fmt.Errorf("arg %d must be numeric but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeDateFull,
			tik.TokenTypeDateLong,
			tik.TokenTypeDateMedium,
			tik.TokenTypeDateShort,
			tik.TokenTypeTimeFull,
			tik.TokenTypeTimeLong,
			tik.TokenTypeTimeMedium,
			tik.TokenTypeTimeShort:
			if typName, isTime := isTime(pkg, arg); !isTime {
				onSrcErr(pos, fmt.Errorf("arg %d must be time.Time but received: %s",
					idx, typName))
				ok = false
				continue
			}

		case tik.TokenTypeCurrency:
			if typName, isCurrency := p.isCurrencyAmount(pkg, arg); !isCurrency {
				onSrcErr(pos, fmt.Errorf(
					"arg %d must be Currency but received: %s",
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

func iterArgs(call *ast.CallExpr, argOffset int) (iter.Seq[ast.Expr], error) {
	if len(call.Args)+argOffset < 2 {
		return func(yield func(ast.Expr) bool) {}, nil
	}
	isEllipsis := call.Ellipsis.IsValid() // true if passed as slice...
	if isEllipsis && argOffset+1 < len(call.Args) {
		secondArg := call.Args[argOffset+1]
		compositeLit, ok := secondArg.(*ast.CompositeLit)
		if !ok {
			return nil, ErrCantUnpackCompositeLiteral
		}
		return func(yield func(ast.Expr) bool) {
			for _, e := range compositeLit.Elts {
				// each elt is an ast.Expr, could be an identifier, call, etc.
				if !yield(e) {
					break
				}
			}
		}, nil
	}
	return func(yield func(ast.Expr) bool) {
		// Iterate over variadic arguments like: foo.String("msg", a, b, c)
		for _, a := range call.Args[argOffset+1:] {
			if !yield(a) {
				break
			}
		}
	}, nil
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

func (p *Parser) isStringValue(
	pkg *packages.Package, e ast.Expr,
) (actualTypeName string, ok bool) {
	tv, found := pkg.TypesInfo.Types[e]
	if !found || tv.Type == nil {
		return tv.Type.String(), false
	}

	named, ok := tv.Type.(*types.Named)
	if !ok {
		return tv.Type.String(), false
	}

	obj := named.Obj()
	objPkg := obj.Pkg()
	if objPkg == nil || obj.Name() != "String" {
		return tv.Type.String(), false
	}

	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return tv.Type.String(), false
	}

	var hasValue, hasGender bool
	for i := range st.NumFields() {
		f := st.Field(i)
		switch f.Name() {
		case "Value":
			b, ok := f.Type().Underlying().(*types.Basic)
			if ok && b.Kind() == types.String {
				hasValue = true
			}
		case "Gender":
			if p.isTypeTokiGender(f.Type()) {
				hasGender = true
			}
		}
	}

	return tv.Type.String(), hasValue && hasGender
}

func (p *Parser) isTypeTokiGender(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil &&
		obj.Pkg() != nil &&
		obj.Pkg().Path()+"."+obj.Name() == p.genderType // e.g. ".../bundle.Gender"
}

func isInteger(pkg *packages.Package, expr ast.Expr) (actualTypeName string, ok bool) {
	tv, ok := pkg.TypesInfo.Types[expr]
	if !ok || tv.Type == nil {
		return tv.Type.String(), false
	}

	switch t := tv.Type.Underlying().(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Uintptr:
			return tv.Type.String(), true
		}
	}
	return tv.Type.String(), false
}

func isFloat(pkg *packages.Package, expr ast.Expr) (actualTypeName string, ok bool) {
	tv, ok := pkg.TypesInfo.Types[expr]
	if !ok || tv.Type == nil {
		return tv.Type.String(), false
	}

	switch t := tv.Type.Underlying().(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Float32, types.Float64:
			return tv.Type.String(), true
		}
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
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Uintptr, types.Float32, types.Float64:
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

func (p *Parser) isCurrencyAmount(
	pkg *packages.Package, e ast.Expr,
) (actualTypeName string, ok bool) {
	tv, found := pkg.TypesInfo.Types[e]
	if !found || tv.Type == nil {
		return tv.Type.String(), false
	}

	named, ok := tv.Type.(*types.Named)
	if !ok {
		return tv.Type.String(), false
	}

	obj := named.Obj()
	objPkg := obj.Pkg()
	if objPkg == nil || obj.Name() != "Currency" {
		return tv.Type.String(), false
	}

	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return tv.Type.String(), false
	}

	var hasValue, hasType bool
	for i := range st.NumFields() {
		f := st.Field(i)
		switch f.Name() {
		case "Amount":
			b, ok := f.Type().Underlying().(*types.Basic)
			if ok && b.Kind() == types.Float64 {
				hasValue = true
			}
		case "Type":
			n, ok := f.Type().(*types.Named)
			if !ok {
				break
			}
			obj := n.Obj()
			pkg := obj.Pkg()
			if obj.Name() == "Type" && pkg != nil &&
				pkg.Path() == "github.com/go-playground/locales/currency" {
				hasType = true
			}
		}
	}

	return tv.Type.String(), hasValue && hasType
}

func isPkgBundle(bundlePkg string, pkg *packages.Package) bool {
	if pkg.Module == nil {
		return false
	}
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
