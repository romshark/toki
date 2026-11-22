package uilang_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/romshark/toki/internal/uilang"
)

func TestFromEnv(t *testing.T) {
	f := func(t *testing.T, env []string, expect ...string) {
		t.Helper()
		var actual []string
		for _, tag := range uilang.FromEnv(env) {
			actual = append(actual, tag.String())
		}
		require.Equal(t, expect, actual)
	}

	f(t, nil)
	f(t, []string{"LANG=de_DE.UTF-8"}, "de-DE")
	f(t, []string{"LANG=uk_UA@euro"}, "uk-UA")
	f(t, []string{"LANG=fr"}, "fr")

	// LC_ALL overrides LC_MESSAGES, which overrides LANG.
	f(t, []string{"LANG=de_DE.UTF-8", "LC_MESSAGES=fr_FR.UTF-8"}, "fr-FR")
	f(t, []string{"LANG=de_DE", "LC_MESSAGES=fr_FR", "LC_ALL=ru_RU"}, "ru-RU")
	f(t, []string{"LC_ALL=", "LANG=de_DE"}, "de-DE")

	// LANGUAGE comes first, but not for C, POSIX or no locale.
	f(t, []string{"LANG=de_DE.UTF-8", "LANGUAGE=uk:fr"}, "uk", "fr", "de-DE")
	f(t, []string{"LANG=C", "LANGUAGE=uk"})
	f(t, []string{"LANG=C.UTF-8", "LANGUAGE=uk"})
	f(t, []string{"LC_ALL=POSIX", "LANG=de_DE"})
	f(t, []string{"LANGUAGE=uk"})

	// The last setting counts.
	f(t, []string{"LANG=de_DE", "LANG=fr_FR"}, "fr-FR")
}

func TestReader(t *testing.T) {
	f := func(t *testing.T, expect string, tags ...language.Tag) {
		t.Helper()
		require.Equal(t, expect, uilang.Reader(tags...).Locale().String())
	}

	f(t, "en")
	f(t, "de", language.MustParse("de-AT"))
	f(t, "uk", language.Japanese, language.Ukrainian)
	// A language without a catalog falls back to the default,
	// not to the first catalog.
	f(t, "en", language.Japanese)
}
