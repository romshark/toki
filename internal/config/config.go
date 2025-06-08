package config

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/text/language"
)

type ConfigGenerate struct {
	Locale        language.Tag
	Translations  []language.Tag
	ModPath       string
	TrimPath      bool
	JSON          bool
	QuietMode     bool
	VerboseMode   bool
	BundlePkgPath string
}

var ErrLocaleNotBCP47 = errors.New("must be a valid BCP 47 locale")

// ParseCLIArgsGenerate parses CLI arguments for command "generate"
func ParseCLIArgsGenerate(osArgs []string) (*ConfigGenerate, error) {
	c := &ConfigGenerate{}

	var locale string
	var translations strArray

	cli := flag.NewFlagSet(osArgs[0], flag.ExitOnError)
	cli.StringVar(&locale, "l", "",
		"default locale of the original source code texts in BCP 47")
	cli.Var(&translations, "t",
		"translation locale in BCP 47 "+
			"(multiple are accepted and duplicates are ignored). "+
			"Creates new catalogs for locales for which no catalogs exist yet.")
	cli.StringVar(&c.ModPath, "m", ".", "path to Go module")
	cli.BoolVar(&c.TrimPath, "trimpath", true, "enable source code path trimming")
	cli.BoolVar(&c.JSON, "json", false, "enables JSON output")
	cli.BoolVar(&c.QuietMode, "q", false, "disable all console logging")
	cli.BoolVar(&c.VerboseMode, "v", false, "enables verbose console logging")
	cli.StringVar(&c.BundlePkgPath, "b", "tokibundle",
		"path to generated Go bundle package relative to module path (-m)")

	if err := cli.Parse(osArgs[2:]); err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	if locale != "" {
		var err error
		c.Locale, err = language.Parse(locale)
		if err != nil {
			return nil, fmt.Errorf(
				"argument l=%q: %w: %w", locale, ErrLocaleNotBCP47, err,
			)
		}
	}

	slices.Sort(translations)
	translations = slices.Compact(translations)
	// Ignore any duplicate of locale in translations.
	// It will be filtered out later during missing catalog detection.
	// We don't do it here because -l is optional when the bundle package exists.
	c.Translations = make([]language.Tag, len(translations))
	for i, s := range translations {
		var err error
		c.Translations[i], err = language.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("argument t=%q: %w: %w", s, ErrLocaleNotBCP47, err)
		}
	}

	return c, nil
}

type strArray []string

func (l *strArray) String() string {
	return strings.Join(*l, ",")
}

func (l *strArray) Set(value string) error {
	*l = append(*l, value)
	return nil
}
