package app

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/gengo"
	"github.com/romshark/toki/internal/log"
	"github.com/romshark/toki/internal/sync"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
	"golang.org/x/text/language"
)

// Generate implements both the `toki generate` and the `toki lint` commands.
type Generate struct {
	hasher           *xxhash.Digest
	icuTokenizer     *icumsg.Tokenizer
	tikParser        *tik.Parser
	tikICUTranslator *tik.ICUTranslator
}

func (g *Generate) Run(
	osArgs []string, lintOnly bool, stderr io.Writer, now time.Time,
) (result Result) {
	result.Start = now
	conf, err := config.ParseCLIArgsGenerate(osArgs)
	if err != nil {
		result.Err = fmt.Errorf("%w: %w", ErrInvalidCLIArgs, err)
		return result
	}
	result.Config = conf

	log.SetWriter(stderr, conf.JSON)

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

	if !lintOnly {
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
	result.Scan, result.Err = parser.Parse(
		conf.ModPath, conf.BundlePkgPath, conf.Locale, conf.TrimPath,
	)
	if result.Err != nil {
		result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, result.Err)
		return result
	}
	scan := result.Scan
	if scan.SourceErrors.Len() > 0 {
		result.Err = ErrSourceErrors
		return result
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
	}

	var nativeCatalog *codeparse.Catalog
	var nativeARB *arb.File
	var nativeARBFileName string
	for catalog := range scan.Catalogs.Seq() {
		if catalog.ARB.Locale == scan.DefaultLocale {
			nativeCatalog = catalog
			nativeARB = catalog.ARB
			nativeARBFileName = catalog.ARBFileName
		}
	}

	if nativeARB == nil {
		nativeARBFileName = filepath.Join(
			conf.BundlePkgPath,
			gengo.FileNameWithLocale(scan.DefaultLocale, "catalog", ".arb"),
		)
		// Make a new one.
		nativeARB = &arb.File{
			Locale:       scan.DefaultLocale,
			LastModified: now,
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
			newMsg, err := g.newARBMsg(scan.DefaultLocale, text)
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
			if catalog.ARB.Locale == scan.DefaultLocale {
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
			newMsg, err := g.newARBMsg(scan.DefaultLocale, text)
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

		if err := writeMissingARBFilesAndUpdateCatalogs(
			now, conf.BundlePkgPath, scan.DefaultLocale, conf.Translations,
			nativeARB, scan.Catalogs,
		); err != nil {
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

func writeARBFiles(bundlePkgPath string, catalogs *sync.Slice[*codeparse.Catalog]) error {
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

func writeMissingARBFilesAndUpdateCatalogs(
	now time.Time, bundlePkgPath string, defaultLocale language.Tag,
	translations []language.Tag, nativeARB *arb.File,
	catalogs *sync.Slice[*codeparse.Catalog],
) error {
	missing := make(map[language.Tag]struct{}, len(translations))
	for _, tr := range translations {
		missing[tr] = struct{}{}
	}
	delete(missing, defaultLocale) // Ignore the default locale, it's the native catalog.

	for catalog := range catalogs.Seq() {
		// Ignore locales of already existing catalogs.
		delete(missing, catalog.ARB.Locale)
	}
	for locale := range missing {
		log.Info("generate new catalog", slog.String("locale", locale.String()))

		name := gengo.FileNameWithLocale(locale, "catalog", ".arb")
		filePath := filepath.Join(bundlePkgPath, name)

		err := func() error {
			f, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return fmt.Errorf("opening new .arb catalog (%q): %w",
					locale.String(), err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					log.Error("closing new catalog .arb file", err)
				}
			}()

			newFile := nativeARB.Copy(func(m *arb.Message) {
				// Reset the message, don't copy from native.
				m.ICUMessage = ""
				m.ICUMessageTokens = nil
			})
			newFile.Locale = locale
			newFile.LastModified = now
			setARBMetadata(newFile)
			if err := arb.Encode(f, newFile, "\t"); err != nil {
				return fmt.Errorf("encoding new .arb catalog (%q): %w",
					locale.String(), err)
			}

			// Add a new catalog.
			newCatalog := &codeparse.Catalog{
				ARB:         newFile,
				ARBFileName: filePath,
			}
			newCatalog.MessagesIncomplete.Store(int64(len(newFile.Messages)))
			catalogs.Append(newCatalog)

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
