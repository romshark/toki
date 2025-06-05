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
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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
	ErrSourceErrors       = errors.New("source code contains errors")
	ErrNoCommand          = errors.New("no command")
	ErrUnknownCommand     = errors.New("unknown command")
	ErrAnalyzingSource    = errors.New("analyzing sources")
	ErrInvalidCLIArgs     = errors.New("invalid arguments")
	ErrMissingLocaleParam = errors.New(
		"please provide a valid BCP 47 locale for the default language of your " +
			"original code base using the 'l' parameter",
	)
)

const Version = "0.4.1"

const MainBundleFileGo = "bundle_gen.go"

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

	log.SetWriter(os.Stderr, conf.JSON)

	switch {
	case conf.QuietMode:
		log.SetMode(log.ModeQuiet) // Disable logging.
	case conf.VerboseMode:
		log.SetMode(log.ModeVerbose)
	}

	if lintOnly {
		log.Info("linting mode")
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

	{
		mainBundleFile := filepath.Join(
			conf.ModPath, conf.BundlePkgPath, MainBundleFileGo,
		)
		if _, err := os.Stat(mainBundleFile); errors.Is(err, os.ErrNotExist) {
			// Require the locale parameter in this case.
			if conf.Locale == language.Und {
				result.Err = ErrMissingLocaleParam
				return result
			}
			scan := &codeparse.Scan{
				DefaultLocale: conf.Locale,
				TokiVersion:   Version,
				Texts:         sync.NewSlice[codeparse.Text](0),
				TextIndexByID: sync.NewMap[string, int](0),
				SourceErrors:  sync.NewSlice[codeparse.SourceError](0),
				Catalogs:      sync.NewSlice[*codeparse.Catalog](0),
			}
			// Need to generate an empty bundle package first.
			// Otherwise if the bundle existed and was imported before, later got removed
			// and then toki generate was rerun it will first generate an incorrect bundle
			// codeparse will be missing method receiver type information on first scan.
			if err := generateGoBundle(conf.BundlePkgPath, scan, headTxt); err != nil {
				result.Err = err
				return result
			}
		} else if err != nil {
			result.Err = fmt.Errorf(
				"checking main bundle file %q: %w",
				mainBundleFile, err,
			)
			return result
		}
	}

	parser := codeparse.NewParser(g.hasher, g.tikParser, g.tikICUTranslator)

	// Parse source code and bundle.
	scan, err := parser.Parse(
		conf.ModPath, conf.BundlePkgPath, conf.Locale, conf.TrimPath,
	)
	if err != nil {
		result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
		return result
	}
	result.Scan = scan
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

	if conf.Locale != language.Und {
		// Locale parameter provided.
		if scan.DefaultLocale != language.Und && conf.Locale != scan.DefaultLocale {
			// The bundle package already existed
			// but the locale parameter doesn't match its default locale.
			result.Err = fmt.Errorf("parameter -l (%q) must either match "+
				"DefaultLocale (%q) in package %q or not be set at all",
				conf.Locale.String(), scan.DefaultLocale.String(), conf.BundlePkgPath)
			return result
		}
		scan.DefaultLocale = conf.Locale
	} else {
		// Locale parameter not provided.
		if scan.DefaultLocale == language.Und {
			// Bundle didn't exist yet so the locale parameter must be provided.
			result.Err = ErrMissingLocaleParam
			return result
		}
		// Use the default locale of the bundle.
		conf.Locale = scan.DefaultLocale
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
			Messages:     make(map[string]arb.Message, scan.TextIndexByID.Len()),
		}
		setARBMetadata(nativeARB)

		nativeCatalog = &codeparse.Catalog{
			ARB:         nativeARB,
			ARBFileName: nativeARBFileName,
		}
		scan.Catalogs.Append(nativeCatalog)
	}

	// Check for new messages.
	for id, index := range scan.TextIndexByID.Seq() {
		if _, ok := nativeARB.Messages[id]; !ok {
			// Text not in native catalog.
			text := scan.Texts.At(index)
			result.NewTexts = append(result.NewTexts, text)
			newMsg, err := g.newARBMsg(conf.Locale, text)
			if err != nil {
				result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
				return result
			}
			log.Verbose("new TIK",
				slog.String("position", log.FmtPos(text.Position)),
				slog.String("id", newMsg.ID))
			nativeARB.Messages[newMsg.ID] = newMsg
			if incomplete := codeparse.IsMsgIncomplete(
				scan, nativeARB, nativeARBFileName, &newMsg,
			); incomplete {
				nativeCatalog.MessagesIncomplete.Add(1)
			}
			continue
		}
		for catalog := range scan.Catalogs.Seq() {
			// Check in all other catalogs.
			if catalog.ARB.Locale == conf.Locale {
				// Skip native catalog. It was already handled above.
				continue
			}
			if _, ok := catalog.ARB.Messages[id]; ok {
				continue
			}
			log.Warn("message missing in catalog",
				slog.String("catalog", catalog.ARB.Locale.String()),
				slog.String("id", id))
			// Message missing in this catalog.
			text := scan.Texts.At(index)
			newMsg, err := g.newARBMsg(conf.Locale, text)
			if err != nil {
				result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
				return result
			}
			catalog.ARB.Messages[newMsg.ID] = arb.Message{
				// Omit the ICU message since we can't take the message
				// generated from the TIK.
				ID:               newMsg.ID,
				Description:      newMsg.Description,
				Comment:          newMsg.Comment,
				Type:             newMsg.Type,
				Context:          newMsg.Context,
				CustomAttributes: newMsg.CustomAttributes,
			}
		}
	}

	// Delete unused messages.
	for id := range nativeARB.Messages {
		index, ok := scan.TextIndexByID.Get(id)
		if ok {
			continue
		}
		text := scan.Texts.At(index)
		log.Verbose("unused TIK",
			slog.String("id", text.IDHash),
			slog.String("pos", log.FmtPos(text.Position)),
		)
		delete(nativeARB.Messages, id)
		// Delete message in all other catalogs too.
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
		if err := generateGoBundle(conf.BundlePkgPath, scan, headTxt); err != nil {
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
						incompletePart := tok.String(msg.ICUMessage, msg.ICUMessageTokens)
						log.Warn("ICU message incomplete",
							slog.String("id", msg.ID),
							slog.String("type", tok.Type.String()),
							slog.String("part", incompletePart))
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
					log.Error("closing catalog .arb file", err)
				}
			}()
			setARBMetadata(catalog.ARB)
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

func setARBMetadata(f *arb.File) {
	if f.CustomAttributes == nil {
		f.CustomAttributes = make(map[string]any, 2)
	}
	f.CustomAttributes["@@x-generator"] = "github.com/romshark/toki"
	f.CustomAttributes["@@x-generator-version"] = Version
}

func prepareBundlePackageDir(bundlePkgPath string) error {
	if _, err := os.Stat(bundlePkgPath); errors.Is(err, os.ErrNotExist) {
		log.Verbose("create new bundle package", slog.String("path", bundlePkgPath))
	}
	if err := os.MkdirAll(bundlePkgPath, 0o755); err != nil {
		return fmt.Errorf("mkdir: bundle package path: %w", err)
	}
	return nil
}

func generateGoBundle(
	bundlePkgPath string, scan *codeparse.Scan, headTxtLines []string,
) error {
	pkgName := filepath.Base(bundlePkgPath)

	// Make sure the bundle package dir exists.
	if err := os.MkdirAll(bundlePkgPath, 0o755); err != nil {
		return err
	}

	bundleGoFilePath := filepath.Join(bundlePkgPath, MainBundleFileGo)
	writer := gengo.NewWriter(Version, scan)
	{
		f, err := os.OpenFile(bundleGoFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}

		// Generate the main Go bundle file.
		var buf bytes.Buffer
		writer.WritePackageBundle(&buf, pkgName, headTxtLines)

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
			writer.WritePackageCatalog(&buf, locale, pkgName, headTxtLines)

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
			log.Warn("head.txt not found")
			return nil, nil
		}

		log.Warn("head.txt not found, creating a new one")
		f, err := os.Create(headFilePath)
		if err != nil {
			return nil, fmt.Errorf("creating head.txt file: %w", err)
		}
		if err := f.Close(); err != nil {
			log.Error("closing head.txt file", err)
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
				log.Error("source errors", nil, slog.Int("total", l))
				for _, e := range s {
					log.Error("source", e.Err, slog.String("pos", log.FmtPos(e.Position)))
				}
			}
			return nil
		})

		fields := []any{
			slog.Int("tiks.total", r.Scan.Texts.Len()),
			slog.Int("tiks.unique", r.Scan.TextIndexByID.Len()),
			slog.Int("tiks.new", len(r.NewTexts)),
			slog.Int("tiks.removed", len(r.RemovedTexts)),
			slog.Int64("scan.files", r.Scan.FilesTraversed.Load()),
			slog.String("scan.duration", time.Since(r.Start).String()),
			slog.Int64("catalogs", int64(r.Scan.Catalogs.Len())),
		}
		_ = r.Scan.Catalogs.Access(func(s []*codeparse.Catalog) error {
			for _, c := range s {
				fieldName := gengo.FileNameWithLocale(c.ARB.Locale, "catalog", ".arb")
				completeness := completeness(c) * 100
				fields = append(fields, slog.Group(fieldName,
					slog.String("completeness", fmt.Sprintf("%.2f%%", completeness))))
			}
			return nil
		})
		log.Info("finished", fields...)
	}
	if r.Err != nil {
		log.Error(r.Err.Error(), nil)
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
) (msg arb.Message, err error) {
	icuMsg := g.tikICUTranslator.TIK2ICU(text.TIK, nil)

	description := strings.Join(text.Comments, " ")

	msg.ID = codeparse.HashMessage(g.hasher, text.TIK.Raw)

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
		case tik.TokenTypeDateFull,
			tik.TokenTypeDateLong,
			tik.TokenTypeDateMedium,
			tik.TokenTypeDateShort:
			pl.Description = "date"
			pl.IsCustomDateFormat = true
			pl.Type = arb.PlaceholderDateTime
		case tik.TokenTypeTimeFull,
			tik.TokenTypeTimeLong,
			tik.TokenTypeTimeMedium,
			tik.TokenTypeTimeShort:
			pl.Description = "time"
			pl.IsCustomDateFormat = true
			pl.Type = arb.PlaceholderDateTime
		case tik.TokenTypeCurrency:
			pl.Description = "currency with amount"
			pl.Type = arb.PlaceholderNum
			pl.Example = "USD(4.00)"
		}
		placeholders[name] = pl
	}

	icuTokens, err := g.icuTokenizer.Tokenize(locale, nil, icuMsg)
	if err != nil {
		return arb.Message{ID: msg.ID}, err
	}

	return arb.Message{
		ID:               msg.ID,
		ICUMessage:       icuMsg,
		ICUMessageTokens: icuTokens,
		Description:      description,
		Type:             arb.MessageTypeText,
		Context:          text.Context(),
		Placeholders:     placeholders,
	}, nil
}
