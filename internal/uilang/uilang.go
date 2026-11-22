// Package uilang picks the language in which Toki talks to its user,
// in the command-line tool and in the editor.
package uilang

import (
	"strings"

	"golang.org/x/text/language"

	"github.com/romshark/toki/internal/tokibundle"
)

// FromEnv returns the languages that the environment env asks for, most
// preferred first, as GNU gettext reads them: the locale is the first of
// LC_ALL, LC_MESSAGES and LANG that is set, and LANGUAGE, a list separated
// by colons, comes before it. It returns none for the locale C or POSIX,
// which ignores LANGUAGE too. As in exec.Cmd.Env, the last setting of a
// variable in env counts.
func FromEnv(env []string) []language.Tag {
	var locale string
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale = lookupEnv(env, name); locale != "" {
			break
		}
	}
	if _, ok := parseLocale(locale); !ok {
		return nil // No locale, C or POSIX, or one that names no language.
	}
	var tags []language.Tag
	for _, v := range append(strings.Split(lookupEnv(env, "LANGUAGE"), ":"), locale) {
		if t, ok := parseLocale(v); ok {
			tags = append(tags, t)
		}
	}
	return tags
}

// lookupEnv returns the value of the last setting of variable name in env.
func lookupEnv(env []string, name string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], name+"="); ok {
			return v
		}
	}
	return ""
}

// parseLocale parses a POSIX locale, as de_DE.UTF-8 or uk_UA@euro, as a language tag.
// It reports false for C, POSIX and values without a language.
func parseLocale(v string) (language.Tag, bool) {
	v, _, _ = strings.Cut(v, ".") // The encoding, as in de_DE.UTF-8.
	v, _, _ = strings.Cut(v, "@") // The modifier, as in uk_UA@euro.
	if v == "" || v == "C" || v == "POSIX" {
		return language.Und, false
	}
	t, err := language.Parse(strings.ReplaceAll(v, "_", "-"))
	if err != nil || t == language.Und {
		return language.Und, false
	}
	return t, true
}

// Reader returns the reader of the catalog that best matches tags, most
// preferred first, and the reader of the default locale when none matches.
func Reader(tags ...language.Tag) tokibundle.Reader {
	reader, confidence := tokibundle.Match(tags...)
	if confidence == language.No {
		// Match then returns the first catalog, which needn't be the default.
		return tokibundle.Default()
	}
	return reader
}
