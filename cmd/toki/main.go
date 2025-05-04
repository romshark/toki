package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"maps"
	"os"
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
	case "lint":
		// TODO: implement lint command
		return Result{
			Err: errors.New("not yet implemented"),
		}, 1
	case "generate":
		g := Generate{
			hasher:           xxhash.New(),
			icuTokenizer:     new(icumsg.Tokenizer),
			tikParser:        tik.NewParser(defaultTIKConfig),
			tikICUTranslator: tik.NewICUTranslator(defaultTIKConfig),
		}
		r := g.Run(osArgs)
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

func (g *Generate) Run(osArgs []string) (result Result) {
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

	// Create bundle package directory if it doesn't exist yet.
	if err := prepareBundlePackageDir(conf.BundlePkgPath); err != nil {
		result.Err = err
		return result
	}

	// Read/create head.txt.
	headTxt, err := readOrCreateHeadTxt(conf)
	if err != nil {
		result.Err = err
		return result
	}

	_ = headTxt // TODO

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

	var nativeARB *arb.File
	for _, catalog := range scan.Catalogs {
		if catalog.ARB.Locale == conf.Locale {
			nativeARB = catalog.ARB
		}
	}
	if nativeARB == nil {
		// Make a new one.
		nativeARB = &arb.File{
			Locale:       conf.Locale,
			LastModified: time.Now(),
			CustomAttributes: map[string]any{
				"x-generator":         "github.com/romshark/toki",
				"x-generator-version": "v0.1.0",
			},
			Messages: make(map[string]arb.Message, len(scan.Texts)),
		}

		nativeCatalog := &codeparse.Catalog{
			ARB: nativeARB,
		}
		nativeCatalog.MessagesIncomplete.Add(
			int64(len(nativeARB.Messages)),
		)
		scan.Catalogs = append(scan.Catalogs, nativeCatalog)
	}

	// Check for new messages
	for _, text := range scan.Texts {
		if _, ok := nativeARB.Messages[text.IDHash]; !ok {
			log.Verbosef("new text at %s:%d:%d\n",
				text.Position.Filename, text.Position.Line, text.Position.Column)
			result.NewTexts = append(result.NewTexts, text)
			id, newMsg, err := g.newARBMsg(conf.Locale, text)
			if err != nil {
				result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
				return result
			}
			nativeARB.Messages[id] = newMsg
			for _, catalog := range scan.Catalogs {
				if catalog.ARB.Locale == conf.Locale {
					// Skip native .arb
					continue
				}
				catalog.ARB.Messages[id] = newMsg
			}
		}
	}

	// (Re-)Generate .arb files.
	if err := writeARBFiles(conf.BundlePkgPath, scan.Catalogs); err != nil {
		result.Err = err
		return result
	}

	// Generate go bundle.
	if err := generateGoBundle(conf.BundlePkgPath, conf.Locale, scan); err != nil {
		result.Err = err
		return result
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
		log.Verbosef("create new bundle package %q", bundlePkgPath)
	}
	if err := os.MkdirAll(bundlePkgPath, 0o755); err != nil {
		return fmt.Errorf("mkdir: bundle package path: %w", err)
	}
	return nil
}

func generateGoBundle(
	bundlePkgPath string, srcLocale language.Tag, scan *codeparse.Scan,
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
		writer.WritePackageBundle(&buf, pkgName, srcLocale, translationLocales)

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
		writer.WritePackageCatalog(&buf, locale, pkgName,
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
func readOrCreateHeadTxt(conf *config.ConfigGenerate) ([]string, error) {
	headFilePath := filepath.Join(conf.BundlePkgPath, "head.txt")
	if fc, err := os.ReadFile(headFilePath); errors.Is(err, os.ErrNotExist) {
		log.Verbosef("head.txt not found, creating a new one")
		f, err := os.Create(headFilePath)
		if err != nil {
			return nil, fmt.Errorf("creating head.txt file: %w", err)
		}
		if err := f.Close(); err != nil {
			log.Errorf("ERR: closing head.txt file: %v\n", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("reading head.txt: %w", err)
	} else {
		return strings.Split(string(fc), "\n"), nil
	}
	return nil, nil
}

type Result struct {
	Config   *config.ConfigGenerate
	Start    time.Time
	Scan     *codeparse.Scan
	NewTexts []codeparse.Text
	Err      error
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
		Texts          int64         `json:"texts"`
		NewTexts       int           `json:"new-texts"`
		FilesTraversed int           `json:"files-traversed"`
		SourceErrors   []SourceError `json:"source-errors,omitempty"`
		TimeMS         int64         `json:"time-ms"`
		Catalogs       []Catalog     `json:"catalogs"`
	}{
		Error:          r.Err,
		Texts:          r.Scan.TextTotal.Load(),
		NewTexts:       len(r.NewTexts),
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
		completeness := float64(1)
		total := float64(len(c.ARB.Messages))
		if total > 0 {
			complete := total - float64(c.MessagesIncomplete.Load())
			completeness = complete / total
		}
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
		log.Errorf("Source errors (%d):\n", len(r.Scan.SourceErrors))
		log.Verbosef("Texts: %d\n", len(r.Scan.Texts))
		log.Verbosef("New texts: %d\n", len(r.NewTexts))
		log.Verbosef("Files traversed: %d\n", r.Scan.FilesTraversed.Load())
		log.Verbosef("Time total: %s\n", time.Since(r.Start).String())
		for _, e := range r.Scan.SourceErrors {
			log.Errorf(" %s:%d:%d: %v\n", e.Filename, e.Line, e.Column, e.Err)
		}
	}
	if r.Err != nil {
		log.Verbosef("Error: %s\n", r.Err.Error())
	}
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
