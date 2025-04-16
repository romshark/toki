package main

import (
	"encoding/json"
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
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
}

func run(osArgs []string) error {
	if len(osArgs) < 2 {
		return fmt.Errorf("%w, use either of: [generate,lint]", ErrNoCommand)
	}
	log.SetLevel(log.LevelDebug)
	switch osArgs[1] {
	case "lint":
		// TODO: implement lint command
		panic("not yet implemented")
	case "generate":
		return runGenerate(osArgs)
	}
	return fmt.Errorf("%w %q, use either of: [generate,lint]",
		ErrUnknownCommand, osArgs[1])
}

func runGenerate(osArgs []string) error {
	start := time.Now()
	conf, err := config.ParseCLIArgsGenerate(osArgs)
	if err != nil {
		return fmt.Errorf("parsing arguments: %w", err)
	}

	if conf.QuietMode {
		log.SetLevel(log.LevelError)
	} else if conf.VerboseMode {
		log.SetLevel(log.LevelVerbose)
	}

	// Read/create head.txt.
	headTxt, err := readOrCreateHeadTxt(conf)
	if err != nil {
		log.Errorf("reading/creating head.txt: %s\n", err.Error())
		return nil
	}

	_ = headTxt // TODO

	parser := codeparse.NewParser()

	// Parse source code and bundle.
	stats, scan, err := parser.Parse(
		conf.SrcPathPattern, conf.BundlePkgPath, conf.Locale, conf.TrimPath,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
	}

	if len(scan.SourceErrors) > 0 {
		log.Errorf("ERRORS (%d):\n", len(scan.SourceErrors))
		for _, e := range scan.SourceErrors {
			log.Errorf("%s:%d:%d: %v\n", e.Filename, e.Line, e.Column, e.Err)
		}
		if err := NewResult(start, stats, scan).Print(conf.JSON); err != nil {
			return err
		}
		return ErrSourceErrors
	}

	// Generate go bundle.
	if err := generateGoBundle(conf.BundlePkgPath, conf.Locale, scan); err != nil {
		return err
	}

	if err := NewResult(start, stats, scan).Print(conf.JSON); err != nil {
		return err
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

func NewResult(
	start time.Time,
	stats *codeparse.Statistics,
	scan *codeparse.Scan,
) *Result {
	se := make([]ResultSourceError, len(scan.SourceErrors))
	for i, e := range scan.SourceErrors {
		se[i] = ResultSourceError{
			Msg: e.Err.Error(),
			Location: ResultSourceLocation{
				File:   e.Filename,
				Line:   e.Line,
				Column: e.Column,
			},
		}
	}
	return &Result{
		TimeTotal:      time.Since(start),
		FilesTraversed: uint64(stats.FilesTraversed.Load()),
		Texts:          uint64(stats.TextTotal.Load()),
	}
}

type ResultSourceError struct {
	Msg      string               `json:"msg"`
	Location ResultSourceLocation `json:"location"`
}

type ResultSourceLocation struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Result struct {
	TimeTotal      time.Duration       `json:"time-total"`
	SourceErrors   []ResultSourceError `json:"source-errors"`
	FilesTraversed uint64              `json:"files-traversed"`
	Texts          uint64              `json:"texts"`
	NewTexts       uint64              `json:"texts-new"`
}

func (r *Result) Print(jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(r)
	}
	log.Verbosef("Texts: %d\n", r.Texts)
	log.Verbosef("Files scanned: %d\n", r.FilesTraversed)
	log.Verbosef("Time total: %s\n", r.TimeTotal.String())
	return nil
}
