package main

import (
	"bufio"
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
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
	"github.com/romshark/toki/internal/sync"
	"golang.org/x/text/language"
)

var (
	ErrSourceErrors    = errors.New("source code contains errors")
	ErrNoCommand       = errors.New("no command")
	ErrUnknownCommand  = errors.New("unknown command")
	ErrAnalyzingSource = errors.New("analyzing sources")
	ErrInvalidCLIArgs  = errors.New("invalid arguments")
)

const Version = "0.3.1"

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
		fmt.Printf("reading build information: %v\n", err)
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

	if conf.Locale != language.Und && scan.DefaultLocale != language.Und &&
		conf.Locale != scan.DefaultLocale {
		// The bundle package already existed
		// but the -l parameter doesn't match its default locale.
		result.Err = fmt.Errorf("parameter -l (%q) must either match DefaultLocale (%q) "+
			"in package %q or not be set at all",
			conf.Locale.String(), scan.DefaultLocale.String(), conf.BundlePkgPath)
		return result
	}

	if scan.SourceErrors.Len() > 0 {
		result.Err = ErrSourceErrors
		return result
	}

	var nativeCatalog *codeparse.Catalog
	var nativeARB *arb.File
	var nativeARBFileName string
	for catalog := range scan.Catalogs.Seq() {
		if catalog.ARB.Locale == conf.Locale {
			nativeCatalog = catalog
			nativeARB = catalog.ARB
			nativeARBFileName = catalog.ARBFileName
		}
	}
	if nativeARB == nil {
		nativeARBFileName = filepath.Join(
			conf.BundlePkgPath,
			gengo.FileNameWithLocale(conf.Locale, "catalog", ".arb"),
		)
		// Make a new one.
		nativeARB = &arb.File{
			Locale:       conf.Locale,
			LastModified: time.Now(),
			CustomAttributes: map[string]any{
				"x-generator":         "github.com/romshark/toki",
				"x-generator-version": "v" + Version,
			},
			Messages: make(map[string]arb.Message, scan.TextIndexByID.Len()),
		}

		nativeCatalog = &codeparse.Catalog{
			ARB:         nativeARB,
			ARBFileName: nativeARBFileName,
		}
		scan.Catalogs.Append(nativeCatalog)
	}

	// Check for new messages
	for text := range scan.Texts.Seq() {
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
			for catalog := range scan.Catalogs.Seq() {
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
		index, ok := scan.TextIndexByID.Get(id)
		if ok {
			continue
		}
		text := scan.Texts.At(index)
		log.Verbosef("unused TIK %s at %s:%d:%d\n",
			text.IDHash, text.Position.Filename,
			text.Position.Line, text.Position.Column,
		)
		delete(nativeARB.Messages, id)
		for c := range scan.Catalogs.Seq() {
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

		if scan.TokiVersion == "" || scan.TokiVersion != Version {
			// Clear generated files on version mismatch.
			if err := deleteAllTokiGeneratedFiles(conf.BundlePkgPath); err != nil {
				result.Err = fmt.Errorf("removing sources of existing bundle: %w", err)
				return result
			}
		}

		// Generate go bundle.
		if err := generateGoBundle(
			conf.BundlePkgPath, conf.Locale, scan, headTxt,
		); err != nil {
			result.Err = err
			return result
		}
	}

	if !conf.QuietMode && conf.VerboseMode {
		// Report incomplete messages in verbose mode.
		for catalog := range scan.Catalogs.Seq() {
			for _, msg := range catalog.ARB.Messages {
				_ = icumsg.Completeness(
					msg.ICUMessage, msg.ICUMessageTokens, catalog.ARB.Locale,
					codeparse.ICUSelectOptions,
					func(index int) {
						tok := msg.ICUMessageTokens[index]
						log.Verbosef("ICU message incomplete: %q: %s %#v\n",
							msg.ID, tok.Type.String(),
							tok.String(msg.ICUMessage, msg.ICUMessageTokens))
					}, // On incomplete.
					func(index int) {
						// On rejected do nothing. This was addressed before this report.
					},
				)
			}
		}
	}

	return result
}

func deleteAllTokiGeneratedFiles(dir string) error {
	const generatedHeader = "// Generated by github.com/romshark/toki. DO NOT EDIT"
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer func() { _ = file.Close() }()

		scanner := bufio.NewScanner(file)
		if scanner.Scan() && strings.HasPrefix(scanner.Text(), generatedHeader) {
			_ = file.Close()
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("deleting file: %w", err)
			}
		}
		return nil
	})
}

func writeARBFiles(
	bundlePkgPath string, catalogs *sync.Slice[*codeparse.Catalog],
) error {
	for catalog := range catalogs.Seq() {
		locale := catalog.ARB.Locale
		name := gengo.FileNameWithLocale(locale, "catalog", ".arb")
		filePath := filepath.Join(bundlePkgPath, name)
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

	// Make sure the bundle package dir exists.
	if err := os.MkdirAll(bundlePkgPath, 0o755); err != nil {
		return err
	}

	bundleGoFilePath := filepath.Join(bundlePkgPath, "bundle_gen.go")
	writer := gengo.NewWriter(Version)
	{
		f, err := os.OpenFile(bundleGoFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}

		// Generate the main Go bundle file.
		var translationLocales []language.Tag
		_ = scan.Catalogs.Access(func(s []*codeparse.Catalog) error {
			translationLocales = make([]language.Tag, len(s))
			for i, catalog := range s {
				translationLocales[i] = catalog.ARB.Locale
			}
			return nil
		})
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
	return scan.Catalogs.Access(func(s []*codeparse.Catalog) error {
		for _, catalog := range s {
			locale := catalog.ARB.Locale
			fileName := gengo.FileNameWithLocale(locale, "catalog", "_gen.go")
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
						txt := scan.Texts.At(scan.TextIndexByID.GetValue(msgID))
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
	})
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
	var errMsg string
	if r.Err != nil {
		errMsg = r.Err.Error()
	}
	data := struct {
		Error          string        `json:"error,omitempty"`
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
		Error:          errMsg,
		StringCalls:    r.Scan.StringCalls.Load(),
		WriteCalls:     r.Scan.WriteCalls.Load(),
		TIKs:           r.Scan.Texts.Len(),
		TIKsUnique:     r.Scan.TextIndexByID.Len(),
		TIKsNew:        len(r.NewTexts),
		FilesTraversed: int(r.Scan.FilesTraversed.Load()),
		TimeMS:         time.Since(r.Start).Milliseconds(),
	}
	_ = r.Scan.SourceErrors.Access(func(s []codeparse.SourceError) error {
		data.SourceErrors = make([]SourceError, len(s))
		for i, serr := range s {
			data.SourceErrors[i] = SourceError{
				Error: serr.Err.Error(),
				File:  serr.Filename,
				Line:  serr.Line,
				Col:   serr.Column,
			}
		}
		return nil
	})
	_ = r.Scan.Catalogs.Access(func(s []*codeparse.Catalog) error {
		data.Catalogs = make([]Catalog, len(s))
		for i, c := range s {
			completeness := completeness(c)
			data.Catalogs[i] = Catalog{
				Locale:       c.ARB.Locale.String(),
				Completeness: completeness,
			}
		}
		return nil
	})
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
		_ = r.Scan.SourceErrors.Access(func(s []codeparse.SourceError) error {
			if l := len(s); l > 0 {
				log.Errorf("Source errors (%d):\n", l)
				for _, e := range s {
					log.Errorf(" %s:%d:%d: %v\n", e.Filename, e.Line, e.Column, e.Err)
				}
			}
			return nil
		})
		log.Verbosef("TIKs: %d (unique: %d; new: %d; removed: %d)\n",
			r.Scan.Texts.Len(), r.Scan.TextIndexByID.Len(),
			len(r.NewTexts), len(r.RemovedTexts))
		log.Verbosef("Scanned %d file(s) in %s\n",
			r.Scan.FilesTraversed.Load(), time.Since(r.Start).String())
		_ = r.Scan.Catalogs.Access(func(s []*codeparse.Catalog) error {
			log.Verbosef("Catalogs (%d):\n", len(s))
			for _, c := range s {
				completeness := completeness(c) * 100
				log.Verbosef(" %s: %.2f%%\n",
					c.ARB.Locale.String(), completeness)
			}
			return nil
		})
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
