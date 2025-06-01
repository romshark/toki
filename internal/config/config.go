package config

import (
	"flag"
	"fmt"

	"golang.org/x/text/language"
)

type ConfigGenerate struct {
	Locale         language.Tag
	Translations   []language.Tag
	SrcPathPattern string
	TrimPath       bool
	JSON           bool
	QuietMode      bool
	VerboseMode    bool
	BundlePkgPath  string
}

// ParseCLIArgsGenerate parses CLI arguments for command "generate"
func ParseCLIArgsGenerate(osArgs []string) (*ConfigGenerate, error) {
	c := &ConfigGenerate{}

	var locale string

	var translationLocales arrayFlags

	cli := flag.NewFlagSet(osArgs[0], flag.ExitOnError)
	cli.Var(&translationLocales, "t", "translation locales")
	cli.StringVar(&locale, "l", "",
		"default locale of the original source code texts in BCP 47")
	cli.StringVar(&c.SrcPathPattern, "p", ".", "path to Go module")
	cli.BoolVar(&c.TrimPath, "trimpath", true, "enable source code path trimming")
	cli.BoolVar(&c.JSON, "json", false, "enables JSON output")
	cli.BoolVar(&c.QuietMode, "q", false, "disable all console logging")
	cli.BoolVar(&c.VerboseMode, "v", false, "enables verbose console logging")
	cli.StringVar(&c.BundlePkgPath, "b", "tokibundle",
		"path to generated Go bundle package relative to module path (-p)")

	if err := cli.Parse(osArgs[2:]); err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	if locale == "" {
		return nil, fmt.Errorf(
			"please provide a valid BCP 47 locale for " +
				"the default language of your original code base " +
				"using the 'l' parameter",
		)
	}

	for _, locale := range translationLocales {
		l, err := language.Parse(locale)
		if err != nil {
			return nil, fmt.Errorf(
				"argument 't' (%q) must be a valid BCP 47 locale: %w", locale, err,
			)
		}
		c.Translations = append(c.Translations, l)
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

type arrayFlags []string

var _ flag.Value = new(arrayFlags)

func (a *arrayFlags) String() string {
	return fmt.Sprintf("%v", *a)
}

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}
