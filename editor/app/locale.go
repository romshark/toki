package app

import (
	"net/http"
	"slices"
	"strings"

	"github.com/romshark/datapages"
	"golang.org/x/text/language"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/internal/tokibundle"
	"github.com/romshark/toki/internal/uilang"
)

// langCookie names the cookie that keeps the interface language the user
// picked on the settings page.
const langCookie = "toki-lang"

// StateLang is the per-tab state of the pages whose event handlers need
// nothing but the language of the tab.
type StateLang struct {
	// Reader renders the text of the tab. StreamOpen sets it.
	Reader tokibundle.Reader
}

// EventLangChanged is "editor.lang_changed"
//
// Dispatched when a tab changes the interface language so every open tab
// renders again in it.
type EventLangChanged struct {
	// Lang is the new language preference, see [isLangPref].
	Lang string `json:"lang"`
}

// isLangPref reports whether pref is a language preference:
// [template.LangSystem] or a locale of the bundle.
func isLangPref(pref string) bool {
	return pref == template.LangSystem || slices.ContainsFunc(
		tokibundle.Locales(),
		func(t language.Tag) bool { return t.String() == pref },
	)
}

// readLangPref returns the language preference that the cookies of r keep.
func readLangPref(r *http.Request) string {
	if c, err := r.Cookie(langCookie); err == nil && isLangPref(c.Value) {
		return c.Value
	}
	return template.LangSystem
}

// readerFor returns the reader of the interface language for r: the one
// the user picked, else the system language, see [App.systemReader].
func (a *App) readerFor(r *http.Request) tokibundle.Reader {
	if pref := readLangPref(r); pref != template.LangSystem {
		return uilang.Reader(language.MustParse(pref))
	}
	return a.systemReader(r)
}

// systemReader returns the reader of the language that the preference
// [template.LangSystem] picks for r: a.SystemReader in the desktop app,
// else the best match for the languages r accepts.
func (a *App) systemReader(r *http.Request) tokibundle.Reader {
	if a.SystemReader != nil {
		return a.SystemReader
	}
	tags, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	return uilang.Reader(tags...)
}

// documentAttrsByLocale holds the attributes of <html> for each locale of
// the bundle. Handlers return them and must not modify them.
var documentAttrsByLocale = func() map[string]datapages.DocumentAttrs {
	m := make(map[string]datapages.DocumentAttrs, len(tokibundle.Locales()))
	for _, t := range tokibundle.Locales() {
		m[t.String()] = datapages.DocumentAttrs{"lang": t.String()}
	}
	return m
}()

// documentAttrs returns the attributes of <html> for a page that reader
// renders.
func documentAttrs(reader tokibundle.Reader) datapages.DocumentAttrs {
	return documentAttrsByLocale[reader.Locale().String()]
}

// OnLangChanged keeps the new language in the cookie and reloads the page,
// which the server renders in the language.
func (PrefsSync) OnLangChanged(e EventLangChanged, sse datapages.SSE) error {
	var script strings.Builder
	writeUIPrefCookieScript(&script, langCookie, e.Lang)
	script.WriteString("location.reload();")
	return sse.ExecuteScript(script.String())
}

// POSTSetLang is /settings/set-lang/{$}
func (PageSettings) POSTSetLang(
	_ *http.Request,
	langChanged datapages.Dispatcher[EventLangChanged],
	signals datapages.Signals[struct {
		Lang string `json:"lang"`
	}],
) error {
	if !isLangPref(signals.Values.Lang) {
		return datapages.ErrBadRequest
	}
	return langChanged.Dispatch(EventLangChanged{Lang: signals.Values.Lang})
}
