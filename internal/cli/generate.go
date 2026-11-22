package cli

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
	"path"
	"path/filepath"
	"strings"
	"time"

	editorapp "github.com/romshark/toki/editor/app"
	"github.com/romshark/toki/internal/agentskills"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/gengo"
	"github.com/romshark/toki/internal/icu"
	"github.com/romshark/toki/internal/log"
	"github.com/romshark/toki/internal/sync"
	tikutil "github.com/romshark/toki/internal/tik"
	"github.com/romshark/toki/internal/tokibundle"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
	"golang.org/x/mod/modfile"
	"golang.org/x/text/language"
)

// Generate implements both the `toki generate` and the `toki lint` commands.
type Generate struct {
	hasher           *xxhash.Digest
	icuTokenizer     *icumsg.Tokenizer
	tikParser        *tik.Parser
	tikICUTranslator *tik.ICUTranslator
}

// Run runs the command of osArgs, logging in the language of tr.
func (g *Generate) Run(
	osArgs, env []string, tr tokibundle.Reader, lintOnly bool,
	stderr, stdout io.Writer, now time.Time,
) (result Result) {
	result.Start = now
	flags := newFlagSet(osArgs[1])
	conf, err := config.ParseCLIArgsGenerate(tr, flags, osArgs[2:])
	if err != nil {
		result.Err = flagError(tr, flags, stdout, err)
		return result
	}
	result.Config = conf

	log.SetWriter(stderr, conf.JSON)
	tr = logReader(tr, conf)

	switch {
	case conf.QuietMode:
		log.SetMode(log.ModeQuiet) // Disable logging.
	case conf.VerboseMode:
		log.SetMode(log.ModeVerbose)
	}

	if lintOnly {
		// Log message of toki lint, which checks without writing files.
		log.Info(tr.String("linting mode"))
	}

	// The bundle package lives in the module, not the working directory.
	bundleDir := filepath.Join(conf.ModPath, conf.BundlePkgPath)

	if !lintOnly {
		// Create bundle package directory if it doesn't exist yet.
		if err := prepareBundlePackageDir(tr, bundleDir); err != nil {
			result.Err = err
			return result
		}
	}

	// Read/create head.txt.
	createIfNotExist := !lintOnly
	headTxt, err := readOrCreateHeadTxt(tr, bundleDir, createIfNotExist)
	if err != nil {
		result.Err = err
		return result
	}

	if !lintOnly {
		// Generate a .tokidomain.yml file at the module root if one doesn't exist.
		if err := generateTokiDomainFile(conf.ModPath); err != nil {
			result.Err = fmt.Errorf("generating %s: %w",
				codeparse.DomainFileName, err)
			return result
		}

		mainBundleFile := filepath.Join(bundleDir, MainBundleFileGo)
		if _, err := os.Stat(mainBundleFile); errors.Is(err, os.ErrNotExist) {
			// Require the locale parameter in this case.
			if conf.Locale == language.Und {
				result.Err = ErrMissingLocaleParam
				return result
			}

			scan := codeparse.NewScan(conf.Locale, Version)
			// Need to generate an empty bundle package first.
			// Otherwise if the bundle existed and was imported before, later got removed
			// and then toki generate was rerun it will first generate an incorrect bundle
			// codeparse will be missing method receiver type information on first scan.
			if err := generateGoBundle(bundleDir, scan, headTxt); err != nil {
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
		env, conf.ModPath, ".", conf.BundlePkgPath, conf.TrimPath,
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

	// Refuse to run if the bundle is corrupt — generate can only fix
	// missing entries, not resolve ARB/TIK disagreements. See `toki repair`.
	var corruptTotal int64
	for c := range scan.Catalogs.SeqRead() {
		corruptTotal += c.MessagesCorrupt.Load()
	}
	if corruptTotal > 0 {
		result.Err = &BundleCorruptError{Corrupt: corruptTotal}
		return result
	}

	if conf.Locale != language.Und {
		// Locale parameter provided.
		if scan.DefaultLocale != language.Und && conf.Locale != scan.DefaultLocale {
			// The bundle package already existed
			// but the locale parameter doesn't match its default locale.
			result.Err = &LocaleMismatchError{
				Flag:      conf.Locale.String(),
				Bundle:    scan.DefaultLocale.String(),
				BundlePkg: conf.BundlePkgPath,
			}
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

	var nativeARB *arb.File
	for catalog := range scan.Catalogs.SeqRead() {
		if catalog.ARB.Locale == scan.DefaultLocale {
			nativeARB = catalog.ARB
		}
	}

	if nativeARB == nil {
		nativeARBFilePath, err := filepath.Abs(filepath.Join(
			bundleDir,
			gengo.FileNameWithLocale(scan.DefaultLocale, "catalog", ".arb"),
		))
		if err != nil {
			panic(err)
		}

		// Make a new one.
		nativeARB = &arb.File{
			Locale:       scan.DefaultLocale,
			LastModified: now,
			Messages:     make(map[string]arb.Message, scan.TextIndexByID.Len()),
		}
		setARBMetadata(nativeARB)

		scan.Catalogs.Append(&codeparse.Catalog{
			ARB:         nativeARB,
			ARBFilePath: nativeARBFilePath,
		})
	}

	// Check for new messages and repair corrupt native messages.
	for id, index := range scan.TextIndexByID.SeqRead() {
		text := scan.Texts.At(index)
		if _, ok := nativeARB.Messages[id]; !ok {
			// Text not in native catalog. Add it.
			result.NewTexts = append(result.NewTexts, text)
			newMsg, err := g.newARBMsg(scan.DefaultLocale, text)
			if err != nil {
				result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
				return result
			}
			// Verbose log message about a TIK that is new in the source code.
			log.Verbose(tr.String("new TIK"),
				slog.String("position", log.FmtPos(text.Position)),
				slog.String("id", newMsg.ID))
			nativeARB.Messages[newMsg.ID] = newMsg
		} else {
			// Repair corrupt native locale messages:
			//  1. Empty - always repopulate from TIK.
			//  2. Locked mismatch - ProducesCompleteICU is true but ICU differs.
			msg := nativeARB.Messages[id]
			expectedICU := g.tikICUTranslator.TIK2ICU(text.TIK)
			needsRepair := msg.ICUMessage == "" ||
				(tikutil.ProducesCompleteICU(scan.DefaultLocale, text.TIK) &&
					msg.ICUMessage != expectedICU)
			if needsRepair {
				newMsg, err := g.newARBMsg(scan.DefaultLocale, text)
				if err != nil {
					result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
					return result
				}
				nativeARB.Messages[id] = newMsg
			}
		}

		// Ensure the message exists in all non-native catalogs.
		for catalog := range scan.Catalogs.SeqRead() {
			if catalog.ARB.Locale == scan.DefaultLocale {
				continue
			}
			if _, ok := catalog.ARB.Messages[id]; ok {
				continue
			}
			// Message missing in this catalog.
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
	for id, msg := range nativeARB.Messages {
		if _, ok := scan.TextIndexByID.Get(id); ok {
			continue
		}
		// The TIK is gone from the source, which leaves only its catalog entry.
		// Verbose log message about a TIK that the source code no longer uses.
		log.Verbose(tr.String("unused TIK"),
			slog.String("id", id),
			slog.String("message", msg.ICUMessage),
		)
		// Delete the message in all catalogs, including the native one.
		for c := range scan.Catalogs.Seq() {
			delete(c.ARB.Messages, id)
		}
		result.RemovedIDs = append(result.RemovedIDs, id)
	}

	// (Re-)Generate .arb files.
	if !lintOnly {
		if err := writeARBFiles(tr, bundleDir, scan.Catalogs); err != nil {
			result.Err = err
			return result
		}

		if err := writeMissingARBFilesAndUpdateCatalogs(
			tr, now, bundleDir, scan.DefaultLocale, conf.Translations,
			nativeARB, scan.Catalogs,
		); err != nil {
			result.Err = err
			return result
		}

		// Generate the Go bundle before clearing the old one
		// to keep it if generation fails.
		files, err := renderGoBundle(bundleDir, scan, headTxt)
		if err != nil {
			result.Err = err
			return result
		}

		if scan.TokiVersion == "" || scan.TokiVersion != Version {
			// Clear generated files on version mismatch.
			if err := deleteAllTokiGeneratedFiles(bundleDir); err != nil {
				result.Err = fmt.Errorf("removing sources of existing bundle: %w", err)
				return result
			}
		}

		if err := writeGoFiles(bundleDir, files); err != nil {
			result.Err = err
			return result
		}

		if conf.AISkills {
			if err := writeAgentSkills(tr, conf, scan.DefaultLocale); err != nil {
				result.Err = fmt.Errorf("writing AI agent skills: %w", err)
				return result
			}
		}
	}

	// Synchronization added and removed messages since parsing counted them.
	for catalog := range scan.Catalogs.SeqRead() {
		catalog.CountIncomplete()
	}

	if !conf.QuietMode && conf.VerboseMode {
		// Report incomplete messages in verbose mode.
		for catalog := range scan.Catalogs.SeqRead() {
			for _, msg := range catalog.ARB.Messages {
				errs := icu.AnalysisReport(catalog.ARB.Locale,
					msg.ICUMessage, msg.ICUMessageTokens,
					codeparse.ICUSelectOptions)
				for _, errMsg := range errs {
					log.Warn(errMsg,
						slog.String("id", msg.ID))
				}
			}
		}
	}

	if conf.RequireComplete && result.Err == nil {
		for catalog := range scan.Catalogs.SeqRead() {
			if catalog.MessagesIncomplete.Load() > 0 {
				result.Err = ErrBundleIncomplete
				return result
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

// writeARBFiles writes the .arb files of catalogs, logging in the language of tr.
func writeARBFiles(
	tr tokibundle.Reader, bundlePkgPath string, catalogs *sync.Slice[*codeparse.Catalog],
) error {
	for catalog := range catalogs.SeqRead() {
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
					// Log message about an error of the file system.
					log.Error(tr.String("closing catalog .arb file"), err)
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

// writeMissingARBFilesAndUpdateCatalogs writes a catalog for each locale of
// translations that has none, logging in the language of tr.
func writeMissingARBFilesAndUpdateCatalogs(
	tr tokibundle.Reader, now time.Time, bundlePkgPath string, defaultLocale language.Tag,
	translations []language.Tag, nativeARB *arb.File,
	catalogs *sync.Slice[*codeparse.Catalog],
) error {
	missing := make(map[language.Tag]struct{}, len(translations))
	for _, t := range translations {
		missing[t] = struct{}{}
	}
	delete(missing, defaultLocale) // Ignore the default locale, it's the native catalog.

	for catalog := range catalogs.SeqRead() {
		// Ignore locales of already existing catalogs.
		delete(missing, catalog.ARB.Locale)
	}
	for locale := range missing {
		// Log message about the catalog of a locale that toki generate creates.
		log.Info(tr.String("generate new catalog"), slog.String("locale", locale.String()))

		name := gengo.FileNameWithLocale(locale, "catalog", ".arb")
		filePath := filepath.Join(bundlePkgPath, name)
		filePath, err := filepath.Abs(filePath)
		if err != nil {
			panic(err)
		}

		err = func() error {
			f, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return fmt.Errorf("opening new .arb catalog (%q): %w",
					locale.String(), err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					// Log message about an error of the file system.
					log.Error(tr.String("closing new catalog .arb file"), err)
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
			catalogs.Append(&codeparse.Catalog{
				ARB:         newFile,
				ARBFilePath: filePath,
			})

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

// generateTokiDomainFile creates a .tokidomain.yml file at modPath if one does not
// already exist. The domain name is derived from the Go module name.
func generateTokiDomainFile(modPath string) error {
	domainFilePath := filepath.Join(modPath, codeparse.DomainFileName)
	if _, err := os.Stat(domainFilePath); err == nil {
		return nil // Already exists, don't overwrite.
	}

	modulePath, err := readModulePath(modPath)
	if err != nil {
		return err
	}
	// Use the last path segment of the module path as the domain name.
	name := path.Base(modulePath)

	content := fmt.Sprintf(
		"# The TIK domain for this directory tree.\n"+
			"# The description provides context for translators working on this domain.\n"+
			"# Also see: https://github.com/romshark/tik/blob/main/SPECIFICATION.md#domains\n"+
			"\n"+
			"# This defines the name of the domain.\n"+
			"name: %s\n"+
			"\n"+
			"# The description provides translators with additional context and instructions on\n"+
			"# how to translate within a directory tree, such as the expected tone, style,\n"+
			"# target audience, or terminology conventions.\n"+
			"description: \"\"\n",
		name,
	)
	return os.WriteFile(domainFilePath, []byte(content), 0o644)
}

// readModulePath returns the module path that the go.mod file in modDir declares.
func readModulePath(modDir string) (string, error) {
	goModPath := filepath.Join(modDir, "go.mod")
	goModData, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("reading go.mod: %w", err)
	}
	mf, err := modfile.Parse(goModPath, goModData, nil)
	if err != nil {
		return "", fmt.Errorf("parsing go.mod: %w", err)
	}
	return mf.Module.Mod.Path, nil
}

// writeAgentSkills writes the skills for AI coding agents into the module
// and logs the files it changed in the language of tr.
func writeAgentSkills(
	tr tokibundle.Reader, conf *config.ConfigGenerate, defaultLocale language.Tag,
) error {
	modulePath, err := readModulePath(conf.ModPath)
	if err != nil {
		return err
	}
	bundleDir := path.Clean(filepath.ToSlash(conf.BundlePkgPath))
	var flags string
	if bundleDir != config.DefaultBundlePkgPath {
		flags = " -b " + bundleDir
	}
	written, err := agentskills.Write(conf.ModPath, agentskills.Project{
		BundleDir:        bundleDir,
		BundleImportPath: path.Join(modulePath, bundleDir),
		DefaultLocale:    defaultLocale.String(),
		NativeCatalog:    gengo.FileNameWithLocale(defaultLocale, "catalog", ".arb"),
		Flags:            flags,
		Version:          releaseVersion(),
	})
	for _, f := range written {
		// Log message about a skill file for AI coding agents that toki wrote.
		log.Info(tr.String("wrote AI agent skill"), slog.String("file", f))
	}
	return err
}

// prepareBundlePackageDir creates the directory of the bundle package,
// logging in the language of tr.
func prepareBundlePackageDir(tr tokibundle.Reader, bundlePkgPath string) error {
	if _, err := os.Stat(bundlePkgPath); errors.Is(err, os.ErrNotExist) {
		// Verbose log message.
		log.Verbose(tr.String("create new bundle package"), slog.String("path", bundlePkgPath))
	}
	if err := os.MkdirAll(bundlePkgPath, 0o755); err != nil {
		return fmt.Errorf("mkdir: bundle package path: %w", err)
	}
	return nil
}

// CleanGenerated deletes stale generated Go files and creates a minimal
// empty bundle so the package compiles for codeparse. This handles the case
// where existing generated code has compile errors.
//
// defaultLocale must be the actual default locale of the bundle
// (not guessed from ARB file order).
func CleanGenerated(bundlePkgPath string, defaultLocale language.Tag) error {
	if err := deleteAllTokiGeneratedFiles(bundlePkgPath); err != nil {
		return fmt.Errorf("deleting stale generated files: %w", err)
	}

	emptyScan := codeparse.NewScan(defaultLocale, Version)
	if err := generateGoBundle(bundlePkgPath, emptyScan, nil); err != nil {
		return fmt.Errorf("generating empty bundle: %w", err)
	}
	return nil
}

// RegenerateBundle runs the same logic as `toki generate` but invoked
// programmatically: walks source code, adds missing native-locale ARB
// entries, ensures every non-native catalog mirrors the set of TIK IDs,
// writes the ARB files, and regenerates the Go bundle. Refuses when the
// bundle is corrupt (see [ErrBundleCorrupt]) — run [RepairBundle] first.
//
// modDir is the absolute path to the Go module root.
// bundlePkgPath is the bundle package path relative to modDir.
func RegenerateBundle(env []string, modDir, bundlePkgPath string) error {
	g := &Generate{
		hasher:           xxhash.New(),
		icuTokenizer:     new(icumsg.Tokenizer),
		tikParser:        tik.NewParser(tik.DefaultConfig),
		tikICUTranslator: tik.NewICUTranslator(tik.DefaultConfig),
	}
	// The editor can't know whether the project runs `toki generate`
	// with -ai-skills=false, which is why it leaves the skills alone.
	osArgs := []string{
		"toki", "generate", "-m", modDir, "-b", bundlePkgPath, "-q", "-ai-skills=false",
	}
	// The run writes nothing: its error goes to the editor.
	r := g.Run(osArgs, env, tokibundle.Default(), false, io.Discard, io.Discard, time.Now())
	return r.Err
}

// GenerateBundle generates Go code from the scan data.
// It reads head.txt from the bundle directory and generates the Go bundle
// and per-locale catalog files.
func GenerateBundle(bundlePkgPath string, scan *codeparse.Scan) error {
	headTxtPath := filepath.Join(bundlePkgPath, "head.txt")
	var headTxtLines []string
	if fc, err := os.ReadFile(headTxtPath); err == nil && len(fc) > 0 {
		headTxtLines = strings.Split(string(fc), "\n")
	}
	return generateGoBundle(bundlePkgPath, scan, headTxtLines)
}

// ApplyChangesAndBuild applies in-memory ARB message edits, writes the
// updated .arb files to disk, cleans stale generated Go, re-parses the
// source (picking up the new ARB content), and regenerates the Go bundle.
// Returns the post-build scan so callers (the editor) can refresh their
// in-memory state. Returns an error without touching any files
// if an edit isn't a valid ICU message.
//
// modDir is the absolute path to the Go module root.
// bundlePkgPath is the bundle package path relative to modDir (e.g. "tokibundle").
// defaultLocale is used by CleanGenerated to produce a minimal valid bundle.
func ApplyChangesAndBuild(
	env []string, modDir, bundlePkgPath string, defaultLocale language.Tag,
	edits []editorapp.BundleEdit,
) (*codeparse.Scan, error) {
	// Generation renders messages from their tokens, not their text.
	var tokenizer icumsg.Tokenizer
	editTokens := make([][]icumsg.Token, len(edits))
	for i, e := range edits {
		locale, err := language.Parse(e.Locale)
		if err != nil {
			return nil, fmt.Errorf("parsing locale of edit %q: %w", e.TIKID, err)
		}
		editTokens[i], err = tokenizer.Tokenize(locale, nil, e.Message)
		if err != nil {
			return nil, fmt.Errorf("parsing edited message %q (%s): %w",
				e.TIKID, e.Locale, err)
		}
	}

	absBundlePkg := filepath.Join(modDir, bundlePkgPath)

	// Clean stale generated Go so codeparse can succeed.
	if err := CleanGenerated(absBundlePkg, defaultLocale); err != nil {
		return nil, fmt.Errorf("cleaning generated files: %w", err)
	}

	// Parse source (reads current .arb files from disk).
	parser := codeparse.NewParser(
		xxhash.New(),
		tik.NewParser(tik.DefaultConfig),
		tik.NewICUTranslator(tik.DefaultConfig),
	)
	scan, err := parser.Parse(env, modDir, "./...", bundlePkgPath, false)
	if err != nil {
		return nil, fmt.Errorf("analyzing source: %w", err)
	}

	// Index ARB files by locale so edits can be applied in-place.
	type arbFileEntry struct {
		*arb.File
		AbsolutePath string
		Changed      bool
	}
	arbFiles := make(map[string]arbFileEntry, scan.Catalogs.Len())
	for c := range scan.Catalogs.Seq() {
		arbFiles[c.ARB.Locale.String()] = arbFileEntry{
			File:         c.ARB,
			AbsolutePath: c.ARBFilePath,
		}
	}
	for i, e := range edits {
		af := arbFiles[e.Locale]
		af.Changed = true
		msg := af.Messages[e.TIKID]
		msg.ICUMessage = e.Message
		msg.ICUMessageTokens = editTokens[i]
		af.Messages[e.TIKID] = msg
		arbFiles[e.Locale] = af
	}

	// Persist edited .arb files.
	for _, af := range arbFiles {
		if !af.Changed {
			continue
		}
		f, err := os.OpenFile(af.AbsolutePath, os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, fmt.Errorf("opening arb file: %w", err)
		}
		err = arb.Encode(f, af.File, "\t")
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("encoding arb file: %w", err)
		}
	}

	// Generate Go from the updated scan.
	if err := GenerateBundle(absBundlePkg, scan); err != nil {
		return nil, fmt.Errorf("generating Go bundle: %w", err)
	}
	return scan, nil
}

// goFile is a generated Go file.
type goFile struct {
	path    string
	content []byte
}

func generateGoBundle(
	bundlePkgPath string, scan *codeparse.Scan, headTxtLines []string,
) error {
	files, err := renderGoBundle(bundlePkgPath, scan, headTxtLines)
	if err != nil {
		return err
	}
	return writeGoFiles(bundlePkgPath, files)
}

// renderGoBundle generates and formats the Go bundle and catalog files
// of scan without touching the existing ones.
func renderGoBundle(
	bundlePkgPath string, scan *codeparse.Scan, headTxtLines []string,
) ([]goFile, error) {
	pkgName := filepath.Base(bundlePkgPath)
	writer := gengo.NewWriter(Version, scan)
	files := make([]goFile, 0, 1+scan.Catalogs.Len())

	var buf bytes.Buffer
	writer.WritePackageBundle(&buf, pkgName, headTxtLines)
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting bundle: %w", err)
	}
	files = append(files, goFile{
		path:    filepath.Join(bundlePkgPath, MainBundleFileGo),
		content: formatted,
	})

	for catalog := range scan.Catalogs.SeqRead() {
		locale := catalog.ARB.Locale
		buf.Reset()
		writer.WritePackageCatalog(&buf, locale, pkgName, headTxtLines)
		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			return nil, fmt.Errorf("formatting package catalog: %w", err)
		}
		fileName := gengo.FileNameWithLocale(locale, "catalog", "_gen.go")
		files = append(files, goFile{
			path:    filepath.Join(bundlePkgPath, fileName),
			content: formatted,
		})
	}
	return files, nil
}

// writeGoFiles writes files into the bundle package directory, creating it if needed.
func writeGoFiles(bundlePkgPath string, files []goFile) error {
	if err := os.MkdirAll(bundlePkgPath, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		if err := os.WriteFile(f.path, f.content, 0o644); err != nil {
			return fmt.Errorf("writing generated code: %w", err)
		}
	}
	return nil
}

// readOrCreateHeadTxt reads the head.txt file if it exists, otherwise creates it.
// It logs in the language of tr.
func readOrCreateHeadTxt(
	tr tokibundle.Reader, bundleDir string, createIfNotExist bool,
) ([]string, error) {
	headFilePath := filepath.Join(bundleDir, "head.txt")
	if fc, err := os.ReadFile(headFilePath); errors.Is(err, os.ErrNotExist) {
		if !createIfNotExist {
			// Log message. Keep head.txt, the name of a file.
			log.Warn(tr.String("head.txt not found"))
			return nil, nil
		}

		// Log message. Keep head.txt, the name of a file.
		log.Warn(tr.String("head.txt not found, creating a new one"))
		f, err := os.Create(headFilePath)
		if err != nil {
			return nil, fmt.Errorf("creating head.txt file: %w", err)
		}
		if err := f.Close(); err != nil {
			// Log message about an error of the file system.
			log.Error(tr.String("closing head.txt file"), err)
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
	icuMsg := g.tikICUTranslator.TIK2ICU(text.TIK)

	description := strings.Join(text.Comments, " ")

	msg.ID = codeparse.HashMessage(g.hasher, text.Domain, text.TIK.Raw)

	placeholders := make(map[string]arb.Placeholder)
	for i, placeholder := range text.TIK.Placeholders() {
		var pl arb.Placeholder
		name := fmt.Sprintf("var%d", i)
		switch placeholder.Type {
		case tik.TokenTypeText:
			pl.Description = "arbitrary string"
			pl.Type = arb.PlaceholderString
		case tik.TokenTypeTextWithGender:
			pl.Description = "arbitrary string with gender information"
			pl.Type = arb.PlaceholderString
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
