package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		r := runGenerate(osArgs)
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

func runGenerate(osArgs []string) (result Result) {
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

	parser := codeparse.NewParser()

	// Parse source code and bundle.
	stats, scan, err := parser.Parse(
		conf.SrcPathPattern, conf.BundlePkgPath, conf.Locale, conf.TrimPath,
	)
	if err != nil {
		result.Err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
		return result
	}
	result.Stats = stats
	result.Scan = scan

	if len(scan.SourceErrors) > 0 {
		result.Err = ErrSourceErrors
		return result
	}

	// Generate go bundle.
	if err := generateGoBundle(conf.BundlePkgPath, conf.Locale, scan); err != nil {
		result.Err = err
		return result
	}

	return result
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
	f, err := os.OpenFile(bundleGoFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	var writer gengo.Writer

	// Generate the main Go bundle file.
	translationLocales := make([]language.Tag, len(scan.ARBFiles))
	for i, arbFile := range scan.ARBFiles {
		translationLocales[i] = arbFile.Locale
	}
	writer.WritePackageBundle(f, pkgName, srcLocale, translationLocales)

	// Generate Go catalog files.
	for _, arbFile := range scan.ARBFiles {
		fileName := fmt.Sprintf("catalog_%s_gen.go", arbFile.Locale.String())
		filePath := filepath.Join(bundlePkgPath, fileName)
		f, err := os.OpenFile(filePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}

		writer.WritePackageCatalog(f, arbFile.Locale, pkgName,
			func(yield func(gengo.Message) bool) {
				for _, m := range arbFile.Messages {
					if !yield(gengo.Message{
						TIK:       m.ID,
						ICUMsg:    m.ICUMessage,
						ICUTokens: m.ICUMessageTokens,
					}) {
						break
					}
				}
			})
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
	Config *config.ConfigGenerate
	Start  time.Time
	Stats  *codeparse.Statistics
	Scan   *codeparse.Scan
	Err    error
}

func (r Result) printJSON() {
	indent := "\t"
	w := os.Stderr
	write := func(s string) { _, _ = fmt.Fprint(w, s) }
	writeLnf := func(indents int, format string, a ...any) {
		for range indents {
			write(indent)
		}
		_, _ = fmt.Fprintf(w, format, a...)
		write("\n")
	}

	writeLnf(0, "{")
	if r.Scan != nil {
		if len(r.Scan.SourceErrors) > 0 {
			writeLnf(1, "%q: [", "source-errors")
			for i, serr := range r.Scan.SourceErrors {
				writeLnf(2, "{")
				writeLnf(3, "%q: %q,", "error", serr.Err.Error())
				writeLnf(3, "%q: %q,", "file", serr.Filename)
				writeLnf(3, "%q: %d,", "line", serr.Line)
				writeLnf(3, "%q: %d", "col", serr.Column)
				if i+1 == len(r.Scan.SourceErrors) {
					// Last entry
					writeLnf(2, "}")
					break
				}
				writeLnf(2, "},")
			}
			writeLnf(1, "],")
		}
	} else {
		writeLnf(1, "%q: null,", "source-errors")
	}
	if r.Stats != nil {
		writeLnf(1, "%q: %d,", "texts", r.Stats.TextTotal.Load())
		writeLnf(1, "%q: %d,", "files", r.Stats.FilesTraversed.Load())
	} else {
		writeLnf(1, "%q: null,", "texts")
		writeLnf(1, "%q: null,", "files")
	}
	writeLnf(1, "%q: %d,", "time-ms", time.Since(r.Start).Milliseconds())
	if r.Err != nil {
		writeLnf(1, "%q: %q", "error", r.Err.Error())
	} else {
		writeLnf(1, "%q: null", "error")
	}
	writeLnf(0, "}")
}

func (r Result) Print() {
	if r.Config != nil && r.Config.JSON {
		r.printJSON()
		return
	}

	if r.Scan != nil {
		log.Errorf("Source errors (%d):\n", len(r.Scan.SourceErrors))
		for _, e := range r.Scan.SourceErrors {
			log.Errorf(" %s:%d:%d: %v\n", e.Filename, e.Line, e.Column, e.Err)
		}
	}
	if r.Stats != nil {
		log.Verbosef("Texts: %d\n", len(r.Scan.Texts))
		log.Verbosef("Files scanned: %d\n", r.Stats.FilesTraversed.Load())
		log.Verbosef("Time total: %s\n", time.Since(r.Start).String())
	}
	if r.Err != nil {
		log.Verbosef("Error: %s\n", r.Err.Error())
	}
}
