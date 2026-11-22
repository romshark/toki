package config

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/text/language"

	"github.com/romshark/toki/internal/tokibundle"
)

// DefaultBundlePkgPath is the bundle package path that the flag -b defaults to.
const DefaultBundlePkgPath = "tokibundle"

type ConfigGenerate struct {
	Locale          language.Tag
	Translations    []language.Tag
	ModPath         string
	TrimPath        bool
	JSON            bool
	QuietMode       bool
	VerboseMode     bool
	BundlePkgPath   string
	RequireComplete bool
	AISkills        bool
}

var ErrLocaleNotBCP47 = errors.New("must be a valid non-und BCP 47 locale")

// LocaleFlagError is the error of a flag whose value isn't a valid non-und
// BCP 47 locale. It is [ErrLocaleNotBCP47].
type LocaleFlagError struct {
	Flag  string // The name of the flag, as l.
	Value string
	Err   error // Why Value doesn't parse, nil for und.
}

func (e *LocaleFlagError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("argument %s=%q: %v: is und", e.Flag, e.Value, ErrLocaleNotBCP47)
	}
	return fmt.Sprintf("argument %s=%q: %v: %v", e.Flag, e.Value, ErrLocaleNotBCP47, e.Err)
}

func (e *LocaleFlagError) Unwrap() []error {
	if e.Err == nil {
		return []error{ErrLocaleNotBCP47}
	}
	return []error{ErrLocaleNotBCP47, e.Err}
}

// parseLocaleFlag parses value, the value of the flag name, as a non-und
// BCP 47 locale.
func parseLocaleFlag(name, value string) (language.Tag, error) {
	t, err := language.Parse(value)
	if err != nil {
		return t, &LocaleFlagError{Flag: name, Value: value, Err: err}
	}
	if t == language.Und {
		return t, &LocaleFlagError{Flag: name, Value: value}
	}
	return t, nil
}

// ParseCLIArgsGenerate defines the flags of the commands "generate" and "lint"
// on cli, describing them in the language of tr, and parses args with it.
func ParseCLIArgsGenerate(
	tr tokibundle.Reader, cli *flag.FlagSet, args []string,
) (*ConfigGenerate, error) {
	c := &ConfigGenerate{}

	var locale string
	var translations strArray

	// The usage of a flag. Keep BCP 47 and "und", the undetermined locale.
	cli.StringVar(&locale, "l", "", tr.String(
		"default locale of the original source code texts in non-und BCP 47"))
	// The usage of a flag. Keep BCP 47 and "und", the undetermined locale.
	cli.Var(&translations, "t", tr.String("translation locale in non-und BCP 47 "+
		"(multiple are accepted and duplicates are ignored). "+
		"Creates new catalogs for locales for which no catalogs exist yet."))
	// The usage of a flag.
	cli.StringVar(&c.ModPath, "m", ".", tr.String("path to Go module"))
	// The usage of a flag.
	cli.BoolVar(&c.TrimPath, "trimpath", true, tr.String("enable source code path trimming"))
	// The usage of a flag.
	cli.BoolVar(&c.JSON, "json", false, tr.String("enables JSON output"))
	// The usage of a flag.
	cli.BoolVar(&c.QuietMode, "q", false, tr.String("disable all console logging"))
	// The usage of a flag.
	cli.BoolVar(&c.VerboseMode, "v", false, tr.String("enables verbose console logging"))
	// The usage of a flag. Keep -m, the flag of the module path.
	cli.StringVar(&c.BundlePkgPath, "b", DefaultBundlePkgPath, tr.String(
		"path to generated Go bundle package relative to module path (-m)"))
	// The usage of a flag.
	cli.BoolVar(&c.RequireComplete, "require-complete", false, tr.String(
		"fails the command if any active catalog has a completeness < 1.0 (under 100%)"))
	// The usage of a flag. Keep the paths .claude/skills and .agents/skills.
	cli.BoolVar(&c.AISkills, "ai-skills", true, tr.String(
		"writes skills for AI coding agents to .claude/skills and .agents/skills "+
			"in the module, replacing earlier versions"))

	if err := cli.Parse(args); err != nil {
		return nil, fmt.Errorf("parsing: %w", err)
	}

	if locale != "" {
		var err error
		if c.Locale, err = parseLocaleFlag("l", locale); err != nil {
			return nil, err
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
		if c.Translations[i], err = parseLocaleFlag("t", s); err != nil {
			return nil, err
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
