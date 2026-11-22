package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/romshark/toki/editor/app"
	"github.com/romshark/toki/internal/uilang"
)

func TestReaderFor(t *testing.T) {
	f := func(t *testing.T, a *app.App, cookie, acceptLanguage, expect string) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if cookie != "" {
			r.Header.Set("Cookie", "toki-lang="+cookie)
		}
		if acceptLanguage != "" {
			r.Header.Set("Accept-Language", acceptLanguage)
		}
		require.Equal(t, expect, a.ReaderFor(r).Locale().String())
	}

	server := app.NewApp("", "", nil, nil)

	// The picked language wins over the browser's.
	f(t, server, "de", "fr-FR,fr;q=0.9", "de")
	f(t, server, "uk", "", "uk")

	// System and invalid picks follow the browser.
	f(t, server, "system", "ru-RU,ru;q=0.9", "ru")
	f(t, server, "xx", "fr-CA", "fr")
	f(t, server, "", "de-AT,en;q=0.5", "de")

	// A browser language without a catalog falls back to the default,
	// not to the first catalog.
	f(t, server, "", "ja", "en")
	f(t, server, "", "", "en")

	// The desktop app follows the language of its terminal,
	// unless the user picks another one.
	desktop := app.NewApp("", "", nil, nil)
	desktop.SystemReader = uilang.Reader(language.Ukrainian)
	f(t, desktop, "", "de-DE", "uk")
	f(t, desktop, "system", "de-DE", "uk")
	f(t, desktop, "fr", "de-DE", "fr")
}
