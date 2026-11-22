package app

import (
	"net/http"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app/datapagesgen/href"
	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/internal/tokibundle"
)

// PageSettings is /settings
type PageSettings struct {
	App *App
	PrefsSync
}

func (p PageSettings) GET(
	r *http.Request,
) (
	body datapages.Component,
	redirect datapages.Redirect,
	docAttrs datapages.DocumentAttrs,
	err error,
) {
	p.App.lock.Lock()
	building := p.App.building
	p.App.lock.Unlock()
	if building {
		return nil, datapages.Redirect{URL: href.PageBuildBundle()}, nil, nil
	}

	tr := p.App.readerFor(r)
	// Sample text of the interface font picker. Write a pangram of your
	// language, a sentence that uses every letter of its alphabet.
	preview := tr.String("The quick brown fox jumps over the lazy dog")
	icuPreview := "{ plural, one {# item} other {# items} }"
	prefs := ReadUIPrefs(r)
	data := template.DataSettingsPreview{
		IsHybrid: p.App.OpenNewWindow != nil,
		Lang:     readLangPref(r),
		Prefs: template.UIPrefs{
			Theme:          prefs.Theme,
			UIFont:         prefs.UIFont,
			EditorFont:     prefs.EditorFont,
			UIFontSize:     prefs.UIFontSize,
			EditorFontSize: prefs.EditorFontSize,
		},
		UIFonts: []template.FontOption{
			{
				Value:   "system",
				Family:  fontFamilies["system"],
				Label:   tr.String("System Default"),
				Preview: preview,
			},
			{
				Value:   "georgia",
				Family:  "Georgia, Times New Roman, serif",
				Label:   "Georgia",
				Preview: preview,
			},
			{
				Value:   "helvetica",
				Family:  "Helvetica Neue, Helvetica, Arial, sans-serif",
				Label:   "Helvetica",
				Preview: preview,
			},
		},
		EditorFonts: []template.FontOption{
			{
				Value:  "mono-system",
				Family: "ui-monospace, SF Mono, Cascadia Code, monospace",
				// The monospace font of the operating system.
				Label:   tr.String("System Mono"),
				Preview: icuPreview,
			},
			{
				Value:   "mono-firacode",
				Family:  "Fira Code, monospace",
				Label:   "Fira Code",
				Preview: icuPreview,
			},
			{
				Value:   "mono-monaco",
				Family:  "Monaco, Consolas, monospace",
				Label:   "Monaco",
				Preview: icuPreview,
			},
			{
				Value:   "mono-courier",
				Family:  "Courier New, Courier, monospace",
				Label:   "Courier New",
				Preview: icuPreview,
			},
		},
		UIFontSizes:     fontSizeOptions(tr, preview, ""),
		EditorFontSizes: fontSizeOptions(tr, icuPreview, "ui-monospace, monospace"),
		UIPreviewTIK:    "{name, select, other {Welcome, {name}!}}",
		UIPreviewICUEN:  "{name, select, other {Welcome back, {name}!}}",
		UIPreviewICUDE:  "{name, select, other {Willkommen, {name}!}}",
		UIPreviewEditorText: "{count, plural,\n" +
			"  one {You have # new message}\n" +
			"  other {You have # new messages}\n}",
		ServerURL: serverURL(r),
	}
	return template.PageSettings(tr, p.App.Version, data), redirect, documentAttrs(tr), nil
}

// fontSizeOptions builds the selectable font sizes, rendering the sample in
// the given family so the editor's sizes preview in a monospace face.
func fontSizeOptions(
	tr tokibundle.Reader, preview, family string,
) []template.FontSizeOption {
	steps := []struct{ value, size, label string }{
		{"very-small", "0.8rem", tr.String("[font size] Very Small")},
		{"small", "0.9rem", tr.String("[font size] Small")},
		{"default", "1rem", tr.String("[font size] Default")},
		{"big", "1.1rem", tr.String("[font size] Big")},
		{"bigger", "1.25rem", tr.String("[font size] Bigger")},
	}
	opts := make([]template.FontSizeOption, 0, len(steps))
	for _, s := range steps {
		opts = append(opts, template.FontSizeOption{
			Value:   s.value,
			Size:    s.size,
			Family:  family,
			Label:   s.label,
			Preview: preview,
		})
	}
	return opts
}

// serverURL returns the absolute URL this server is reachable at, derived
// from the incoming request. Useful for showing a "open externally" link
// inside the Wails desktop webview.
func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// POSTSetPref is /settings/set-pref/{$}
func (p PageSettings) POSTSetPref(
	_ *http.Request,
	prefsChanged datapages.Dispatcher[EventPrefsChanged],
	signals datapages.Signals[PrefSignals],
) error {
	prefSignals := signals.Values.Normalized()
	if !prefSignals.Valid() {
		return datapages.ErrBadRequest
	}
	p2 := prefSignals.UIPrefs()
	return prefsChanged.Dispatch(EventPrefsChanged{
		Theme:          p2.Theme,
		ThemeResolved:  prefSignals.PrefThemeResolved,
		UIFont:         p2.UIFont,
		EditorFont:     p2.EditorFont,
		UIFontSize:     p2.UIFontSize,
		EditorFontSize: p2.EditorFontSize,
	})
}
