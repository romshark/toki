package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
	"github.com/romshark/toki/editor/datapagesgen/httperr"
)

// PageSettings is /settings
type PageSettings struct{ App *App }

func (p PageSettings) GET(
	r *http.Request,
) (body templ.Component, redirect string, err error) {
	p.App.mu.Lock()
	building := p.App.building
	p.App.mu.Unlock()
	if building {
		return nil, href.PageBuildBundle(), nil
	}

	preview := "The quick brown fox jumps over the lazy dog"
	icuPreview := "{ plural, one {# item} other {# items} }"
	prefs := ReadUIPrefs(r)
	data := template.DataSettingsPreview{
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
				Label:   "System Default",
				Preview: preview,
			},
			{
				Value:   "georgia",
				Family:  "Georgia, 'Times New Roman', serif",
				Label:   "Georgia",
				Preview: preview,
			},
			{
				Value:   "helvetica",
				Family:  "'Helvetica Neue', Helvetica, Arial, sans-serif",
				Label:   "Helvetica",
				Preview: preview,
			},
		},
		EditorFonts: []template.FontOption{
			{
				Value:   "mono-system",
				Family:  "ui-monospace, 'SF Mono', 'Cascadia Code', monospace",
				Label:   "System Mono",
				Preview: icuPreview,
			},
			{
				Value:   "mono-firacode",
				Family:  "'Fira Code', monospace",
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
				Family:  "'Courier New', Courier, monospace",
				Label:   "Courier New",
				Preview: icuPreview,
			},
		},
		UIPreviewTIK:   "{name, select, other {Welcome, {name}!}}",
		UIPreviewICUEN: "{name, select, other {Welcome back, {name}!}}",
		UIPreviewICUDE: "{name, select, other {Willkommen, {name}!}}",
		UIPreviewEditorText: "{count, plural,\n" +
			"  one {You have # new message}\n" +
			"  other {You have # new messages}\n}",
	}
	body = template.PageSettings(p.App.Version, data)
	return
}

// POSTSetPref is /settings/set-pref/{$}
func (p PageSettings) POSTSetPref(
	_ *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		PrefTheme          string `json:"pref_theme"`
		PrefThemeResolved  string `json:"pref_theme_resolved"`
		PrefUIFont         string `json:"pref_ui_font"`
		PrefEditorFont     string `json:"pref_editor_font"`
		PrefUIFontSize     string `json:"pref_ui_font_size"`
		PrefEditorFontSize string `json:"pref_editor_font_size"`
	},
) error {
	prefSignals := settingsPrefSignals{
		PrefTheme:          signals.PrefTheme,
		PrefThemeResolved:  signals.PrefThemeResolved,
		PrefUIFont:         signals.PrefUIFont,
		PrefEditorFont:     signals.PrefEditorFont,
		PrefUIFontSize:     signals.PrefUIFontSize,
		PrefEditorFontSize: signals.PrefEditorFontSize,
	}
	prefSignals = prefSignals.Normalized()
	if !prefSignals.Valid() {
		return httperr.BadRequest
	}
	prefs := prefSignals.UIPrefs()
	signalsOut := prefs.Signals()
	signalsOut.PrefThemeResolved = prefSignals.PrefThemeResolved
	if err := sse.MarshalAndPatchSignals(signalsOut); err != nil {
		return err
	}
	return sse.ExecuteScript(applyUIPrefsScript(prefs))
}
