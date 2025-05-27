package main

import (
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/gengo"
	"github.com/romshark/toki/internal/log"
	"golang.org/x/text/language"
)

var (
	ErrSourceErrors    = errors.New("source code contains errors")
	ErrNoCommand       = errors.New("no command")
	ErrUnknownCommand  = errors.New("unknown command")
	ErrAnalyzingSource = errors.New("analyzing sources")
	ErrInvalidCLIArgs  = errors.New("invalid arguments")
)

const Version = "0.2.2"

func PrintVersionInfoAndExit() {
	defer os.Exit(0)

	p, err := exec.LookPath(os.Args[0])
	if err != nil {
		fmt.Printf("resolving executable file path: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Open(p)
	if err != nil {
		fmt.Printf("opening executable file %q: %v\n", os.Args[0], err)
		os.Exit(1)
	}

	info, err := buildinfo.Read(f)
	if err != nil {
		fmt.Printf("Reading build information: %v\n", err)
	}

	fmt.Printf("Toki v%s\n\n", Version)
	fmt.Printf("%v\n", info)
}

func main() {
	r, exitCode := run(os.Args)
	r.Print()
	os.Exit(exitCode)
}

func run(osArgs []string) (result Result, exitCode int) {
	if len(osArgs) < 2 {
		return Result{
			Err: fmt.Errorf("%w, use either of: [generate,lint]", ErrNoCommand),
		}, 2
	}
	log.SetLevel(log.LevelDebug)
	switch osArgs[1] {
	case "version":
		PrintVersionInfoAndExit()

	case "lint", "generate":
		g := Generate{
			hasher:           xxhash.New(),
			icuTokenizer:     new(icumsg.Tokenizer),
			tikParser:        tik.NewParser(defaultTIKConfig),
			tikICUTranslator: tik.NewICUTranslator(defaultTIKConfig),
		}
		lintOnly := osArgs[1] == "lint"
		r := g.Run(osArgs, lintOnly)
		switch {
		case errors.Is(r.Err, ErrInvalidCLIArgs):
			return r, 2
		case r.Err != nil:
			return r, 1
		}
		return r, 0
	}
	return Result{
		Err: fmt.Errorf("%w %q, use either of: [generate,lint]",
			ErrUnknownCommand, osArgs[1]),
	}, 2
}

var defaultTIKConfig = tik.DefaultConfig()

type Generate struct {
	hasher           *xxhash.Digest
	icuTokenizer     *icumsg.Tokenizer
	tikParser        *tik.Parser
	tikICUTranslator *tik.ICUTranslator
}

func (g *Generate) Run(osArgs []string, lintOnly bool) (result Result) {
	result.Start = time.Now()
	conf, err := config.ParseCLIArgsGenerate(osArgs)
	if err != nil {
		result.Err = fmt.Errorf("%w: %w", ErrInvalidCLIArgs, err)
		return result
	}
	result.Config = conf

	if conf.QuietMode {
		log.SetLevel(log.LevelError)
	} else if conf.VerboseMode {
		log.SetLevel(log.LevelVerbose)
	}

	if lintOnly {
		log.Verbosef("linting only mode\n")
	}

	if !lintOnly {
		// Create bundle package directory if it doesn't exist yet.
		if err := prepareBundlePackageDir(conf.BundlePkgPath); err != nil {
			result.Err = err
			return result
		}
	}

	// Read/create head.txt.
	createIfNotExist := !lintOnly
	headTxt, err := readOrCreateHeadTxt(conf, createIfNotExist)
	if err != nil {
		result.Err = err
		return result
	}

	parser := codeparse.NewParser(g.hasher, g.tikParser, g.tikICUTranslator)

	// Parse source code and bundle.
	bundlePkg := filepath.Base(conf.BundlePkgPath)
	scan, err := parser.Parse(
		conf.SrcPathPattern, bundlePkg, conf.Locale, conf.TrimPath,
	)
	if err != nil {
		result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
		return result
	}
	result.Scan = scan

	if len(scan.SourceErrors) > 0 {
		result.Err = ErrSourceErrors
		return result
	}

	var nativeCatalog *codeparse.Catalog
	var nativeARB *arb.File
	var nativeARBFileName string
	for _, catalog := range scan.Catalogs {
		if catalog.ARB.Locale == conf.Locale {
			nativeCatalog = catalog
			nativeARB = catalog.ARB
			nativeARBFileName = catalog.ARBFileName
		}
	}
	if nativeARB == nil {
		nativeARBFileName = filepath.Join(
			conf.BundlePkgPath,
			"catalog."+gengo.LocaleToCatalogSuffix(conf.Locale)+".arb",
		)
		// Make a new one.
		nativeARB = &arb.File{
			Locale:       conf.Locale,
			LastModified: time.Now(),
			CustomAttributes: map[string]any{
				"x-generator":         "github.com/romshark/toki",
				"x-generator-version": "v" + Version,
			},
			Messages: make(map[string]arb.Message, len(scan.TextIndexByID)),
		}

		nativeCatalog = &codeparse.Catalog{
			ARB:         nativeARB,
			ARBFileName: nativeARBFileName,
		}
		scan.Catalogs = append(scan.Catalogs, nativeCatalog)
	}

	// Check for new messages
	for _, text := range scan.Texts {
		if _, ok := nativeARB.Messages[text.IDHash]; !ok {
			log.Verbosef("new TIK at %s:%d:%d\n",
				text.Position.Filename, text.Position.Line, text.Position.Column)
			result.NewTexts = append(result.NewTexts, text)
			id, newMsg, err := g.newARBMsg(conf.Locale, text)
			if err != nil {
				result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
				return result
			}
			nativeARB.Messages[id] = newMsg
			if incomplete := codeparse.IsMsgIncomplete(
				scan, nativeARB, nativeARBFileName, &newMsg,
			); incomplete {
				nativeCatalog.MessagesIncomplete.Add(1)
			}
			for _, catalog := range scan.Catalogs {
				if catalog.ARB.Locale == conf.Locale {
					// Skip native .arb
					continue
				}
				catalog.ARB.Messages[id] = newMsg
			}
		}
	}

	// Delete unused messages
	for id := range nativeARB.Messages {
		index, ok := scan.TextIndexByID[id]
		if ok {
			continue
		}
		text := scan.Texts[index]
		log.Verbosef("unused TIK %s at %s:%d:%d\n",
			text.IDHash, text.Position.Filename,
			text.Position.Line, text.Position.Column,
		)
		delete(nativeARB.Messages, id)
		for _, c := range scan.Catalogs {
			delete(c.ARB.Messages, id)
		}
		result.RemovedTexts = append(result.RemovedTexts, text)
	}

	// (Re-)Generate .arb files.
	if !lintOnly {
		if err := writeARBFiles(conf.BundlePkgPath, scan.Catalogs); err != nil {
			result.Err = err
			return result
		}

		// Generate go bundle.
		if err := generateGoBundle(
			conf.BundlePkgPath, conf.Locale, scan, headTxt,
		); err != nil {
			result.Err = err
			return result
		}
	}

	return result
}

func writeARBFiles(bundlePkgPath string, catalogs []*codeparse.Catalog) error {
	for _, catalog := range catalogs {
		locale := catalog.ARB.Locale
		loc := strings.ToLower(gengo.LocaleToCatalogSuffix(locale))
		filePath := filepath.Join(bundlePkgPath, fmt.Sprintf("catalog.%s.arb",
			loc))
		err := func() error {
			f, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return fmt.Errorf("opening .arb catalog (%q): %w",
					locale.String(), err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					log.Errorf("closing catalog .arb file: %v", err)
				}
			}()
			if err := arb.Encode(f, catalog.ARB, "\t"); err != nil {
				return fmt.Errorf("encoding .arb catalog (%q): %w",
					locale.String(), err)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func prepareBundlePackageDir(bundlePkgPath string) error {
	if _, err := os.Stat(bundlePkgPath); errors.Is(err, os.ErrNotExist) {
		log.Verbosef("create new bundle package %q\n", bundlePkgPath)
	}
	if err := os.MkdirAll(bundlePkgPath, 0o755); err != nil {
		return fmt.Errorf("mkdir: bundle package path: %w", err)
	}
	return nil
}

func generateGoBundle(
	bundlePkgPath string, srcLocale language.Tag, scan *codeparse.Scan,
	headTxtLines []string,
) error {
	pkgName := filepath.Base(bundlePkgPath)

	bundleGoFilePath := filepath.Join(bundlePkgPath, "bundle_gen.go")
	var writer gengo.Writer
	{
		f, err := os.OpenFile(bundleGoFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}

		// Generate the main Go bundle file.
		translationLocales := make([]language.Tag, len(scan.Catalogs))
		for i, catalog := range scan.Catalogs {
			translationLocales[i] = catalog.ARB.Locale
		}
		var buf bytes.Buffer
		writer.WritePackageBundle(
			&buf, pkgName, srcLocale, translationLocales, headTxtLines,
		)

		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			return fmt.Errorf("formatting bundle: %w", err)
		}
		if _, err := f.Write(formatted); err != nil {
			return fmt.Errorf("writing formatted bundle code to file: %w", err)
		}
	}

	// Generate Go catalog files.
	for _, catalog := range scan.Catalogs {
		locale := catalog.ARB.Locale
		fileName := fmt.Sprintf("catalog_%s_gen.go", locale.String())
		filePath := filepath.Join(bundlePkgPath, fileName)
		f, err := os.OpenFile(filePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		writer.WritePackageCatalog(&buf, locale, pkgName, headTxtLines,
			func(yield func(gengo.Message) bool) {
				sorted := slices.Sorted(maps.Keys(catalog.ARB.Messages))
				for _, msgID := range sorted {
					m := catalog.ARB.Messages[msgID]
					txt := scan.Texts[scan.TextIndexByID[msgID]]
					if !yield(gengo.Message{
						ID:        txt.IDHash,
						TIK:       txt.TIK.Raw,
						ICUMsg:    m.ICUMessage,
						ICUTokens: m.ICUMessageTokens,
					}) {
						break
					}
				}
			})

		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			return fmt.Errorf("formatting package catalog: %w", err)
		}
		if _, err := f.Write(formatted); err != nil {
			return fmt.Errorf("writing formatted code to file: %w", err)
		}
	}
	return nil
}

// readOrCreateHeadTxt reads the head.txt file if it exists, otherwise creates it.
func readOrCreateHeadTxt(
	conf *config.ConfigGenerate,
	createIfNotExist bool,
) ([]string, error) {
	headFilePath := filepath.Join(conf.BundlePkgPath, "head.txt")
	if fc, err := os.ReadFile(headFilePath); errors.Is(err, os.ErrNotExist) {
		if !createIfNotExist {
			log.Verbosef("head.txt not found\n")
			return nil, nil
		}

		log.Verbosef("head.txt not found, creating a new one\n")
		f, err := os.Create(headFilePath)
		if err != nil {
			return nil, fmt.Errorf("creating head.txt file: %w", err)
		}
		if err := f.Close(); err != nil {
			log.Errorf("ERR: closing head.txt file: %v\n", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("reading head.txt: %w", err)
	} else if len(fc) > 0 {
		return strings.Split(string(fc), "\n"), nil
	}
	return nil, nil
}

type Result struct {
	Config       *config.ConfigGenerate
	Start        time.Time
	Scan         *codeparse.Scan
	NewTexts     []codeparse.Text
	RemovedTexts []codeparse.Text
	Err          error
}

func (r Result) mustPrintJSON() {
	enc := json.NewEncoder(os.Stderr)

	type Catalog struct {
		Locale       string  `json:"locale"`
		Completeness float64 `json:"completeness"`
	}
	type SourceError struct {
		Error string `json:"error"`
		File  string `json:"file"`
		Line  int    `json:"line"`
		Col   int    `json:"col"`
	}
	data := struct {
		Error          error         `json:"error,omitempty"`
		StringCalls    int64         `json:"string-calls"`
		WriteCalls     int64         `json:"write-calls"`
		TIKs           int           `json:"tiks"`
		TIKsUnique     int           `json:"tiks-unique"`
		TIKsNew        int           `json:"tiks-new"`
		FilesTraversed int           `json:"files-traversed"`
		SourceErrors   []SourceError `json:"source-errors,omitempty"`
		TimeMS         int64         `json:"time-ms"`
		Catalogs       []Catalog     `json:"catalogs"`
	}{
		Error:          r.Err,
		StringCalls:    r.Scan.StringCalls.Load(),
		WriteCalls:     r.Scan.WriteCalls.Load(),
		TIKs:           len(r.Scan.Texts),
		TIKsUnique:     len(r.Scan.TextIndexByID),
		TIKsNew:        len(r.NewTexts),
		FilesTraversed: int(r.Scan.FilesTraversed.Load()),
		SourceErrors:   make([]SourceError, len(r.Scan.SourceErrors)),
		TimeMS:         time.Since(r.Start).Milliseconds(),
		Catalogs:       make([]Catalog, len(r.Scan.Catalogs)),
	}
	for i, serr := range r.Scan.SourceErrors {
		data.SourceErrors[i] = SourceError{
			Error: serr.Err.Error(),
			File:  serr.Filename,
			Line:  serr.Line,
			Col:   serr.Column,
		}
	}
	for i, c := range r.Scan.Catalogs {
		completeness := completeness(c)
		data.Catalogs[i] = Catalog{
			Locale:       c.ARB.Locale.String(),
			Completeness: completeness,
		}
	}
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		panic(fmt.Errorf("encoding JSON to stderr: %w", err))
	}
}

func (r Result) Print() {
	if r.Config != nil && r.Config.JSON {
		r.mustPrintJSON()
		return
	}

	if r.Scan != nil {
		if l := len(r.Scan.SourceErrors); l > 0 {
			log.Errorf("Source errors (%d):\n", l)
			for _, e := range r.Scan.SourceErrors {
				log.Errorf(" %s:%d:%d: %v\n", e.Filename, e.Line, e.Column, e.Err)
			}
		}
		log.Verbosef("TIKs: %d (unique: %d; new: %d; removed: %d)\n",
			len(r.Scan.Texts), len(r.Scan.TextIndexByID),
			len(r.NewTexts), len(r.RemovedTexts))
		log.Verbosef("Scanned %d file(s) in %s\n",
			r.Scan.FilesTraversed.Load(), time.Since(r.Start).String())
		log.Verbosef("Catalogs (%d):\n", len(r.Scan.Catalogs))
		for _, c := range r.Scan.Catalogs {
			completeness := completeness(c) * 100
			log.Verbosef(" %s: %.2f%%\n",
				c.ARB.Locale.String(), completeness)
		}
	}
	if r.Err != nil {
		log.Verbosef("Error: %s\n", r.Err.Error())
	}
}

func completeness(catalog *codeparse.Catalog) float64 {
	c := float64(1)
	total := float64(len(catalog.ARB.Messages))
	if total > 0 {
		complete := total - float64(catalog.MessagesIncomplete.Load())
		c = complete / total
	}
	return c
}

func (g *Generate) newARBMsg(
	locale language.Tag, text codeparse.Text,
) (id string, msg arb.Message, err error) {
	icuMsg := g.tikICUTranslator.TIK2ICU(text.TIK, nil)

	description := strings.Join(text.Comments, " ")

	id = codeparse.HashMessage(g.hasher, text.TIK.Raw)

	placeholders := make(map[string]arb.Placeholder)
	for i, placeholder := range text.TIK.Placeholders() {
		var pl arb.Placeholder
		name := fmt.Sprintf("var%d", i)
		switch placeholder.Type {
		case tik.TokenTypeStringPlaceholder:
			pl.Description = "arbitrary string"
			pl.Type = arb.PlaceholderString
			example := placeholder.String(text.TIK.Raw)
			pl.Example = example[2 : len(example)-2] // Strip away the {""}.
		case tik.TokenTypeGenderPronoun:
			pl.Description = "gender pronoun"
			pl.Type = arb.PlaceholderString
			pl.Example = "female"
		case tik.TokenTypeCardinalPluralStart:
			pl.Description = "cardinal plural"
			pl.Type = arb.PlaceholderNum
			pl.Example = "2"
		case tik.TokenTypeOrdinalPlural:
			pl.Description = "ordinal plural"
			pl.Type = arb.PlaceholderNum
			pl.Example = "4"
		case tik.TokenTypeTimeShort,
			tik.TokenTypeTimeShortSeconds,
			tik.TokenTypeTimeFullMonthAndDay,
			tik.TokenTypeTimeShortMonthAndDay,
			tik.TokenTypeTimeFullMonthAndYear,
			tik.TokenTypeTimeWeekday,
			tik.TokenTypeTimeDateAndShort,
			tik.TokenTypeTimeYear,
			tik.TokenTypeTimeFull:
			pl.Description = "time"
			pl.IsCustomDateFormat = true
			pl.Type = arb.PlaceholderDateTime
			pl.Example = "RFC3339(2025-04-27T21:16:32+00:00)"
		case tik.TokenTypeCurrencyCodeFull, tik.TokenTypeCurrencyFull:
			pl.Description = "currency with amount"
			pl.Type = arb.PlaceholderNum
			pl.Example = "USD(4)"
		case tik.TokenTypeCurrencyRounded, tik.TokenTypeCurrencyCodeRounded:
			pl.Description = "currency with rounded amount"
			pl.Type = arb.PlaceholderNum
			pl.Example = "USD(4)"
		}
		placeholders[name] = pl
	}

	icuTokens, err := g.icuTokenizer.Tokenize(locale, nil, icuMsg)
	if err != nil {
		return id, arb.Message{}, err
	}

	return id, arb.Message{
		ID:               id,
		ICUMessage:       icuMsg,
		ICUMessageTokens: icuTokens,
		Description:      description,
		Type:             arb.MessageTypeText,
		Context:          text.Context(),
		Placeholders:     placeholders,
	}, nil
}
