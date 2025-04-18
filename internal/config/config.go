package config

import (
	"flag"
	"fmt"
	"path/filepath"

	"golang.org/x/text/language"
)

type ConfigGenerate struct {
	Locale                 language.Tag
	SrcPathPattern         string
	OutPathCatalogTemplate string
	TrimPath               bool
	JSON                   bool
	QuietMode              bool
	VerboseMode            bool
	BundlePkgPath          string
}

// ParseCLIArgsGenerate parses CLI arguments for command "generate"
func ParseCLIArgsGenerate(osArgs []string) (*ConfigGenerate, error) {
	c := &ConfigGenerate{}

	var locale string

	cli := flag.NewFlagSet(osArgs[0], flag.ExitOnError)
	cli.StringVar(&locale, "l", "",
		"default locale of the original source code texts in BCP 47")
	cli.StringVar(&c.SrcPathPattern, "p", ".", "path to Go module")
	cli.StringVar(&c.OutPathCatalogTemplate, "tmpl", "",
		"catalog template output file path. Set to bundle package by default.")
	cli.BoolVar(&c.TrimPath, "trimpath", true, "enable source code path trimming")
	cli.BoolVar(&c.JSON, "json", false, "enables JSON output")
	cli.BoolVar(&c.QuietMode, "q", false, "disable all console logging")
	cli.BoolVar(&c.VerboseMode, "v", false, "enables verbose console logging")
	cli.StringVar(&c.BundlePkgPath, "b", "tokibundle",
		"path to generated Go bundle package relative to module path (-p)")

	if err := cli.Parse(osArgs[2:]); err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	if c.OutPathCatalogTemplate == "" {
		c.OutPathCatalogTemplate = catalogTemplateFileName(
			c.BundlePkgPath,
		)
	}

	if locale == "" {
		return nil, fmt.Errorf(
			"please provide a valid BCP 47 locale for " +
				"the default language of your original code base " +
				"using the 'l' parameter",
		)
	}
	var err error
	c.Locale, err = language.Parse(locale)
	if err != nil {
		return nil, fmt.Errorf(
			"argument 'l' (%q) must be a valid BCP 47 locale: %w", locale, err,
		)
	}

	return c, nil
}

func catalogTemplateFileName(outPath string) string {
	return filepath.Join(outPath, "catalog.pot")
}
