package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// UIPrefs holds user interface preferences stored in cookies.
type UIPrefs struct {
	Theme          string // "light", "dark", "system"
	UIFont         string // "system", "georgia", "helvetica"
	EditorFont     string // "mono-system", "mono-firacode", etc.
	UIFontSize     string // "default", "small", "big", etc.
	EditorFontSize string // "default", "small", "big", etc.
}

type settingsPrefSignals struct {
	PrefTheme          string `json:"pref_theme"`
	PrefThemeResolved  string `json:"pref_theme_resolved"`
	PrefUIFont         string `json:"pref_ui_font"`
	PrefEditorFont     string `json:"pref_editor_font"`
	PrefUIFontSize     string `json:"pref_ui_font_size"`
	PrefEditorFontSize string `json:"pref_editor_font_size"`
}

var uiPrefsDefaults = UIPrefs{
	Theme:          "system",
	UIFont:         "system",
	EditorFont:     "mono-system",
	UIFontSize:     "default",
	EditorFontSize: "default",
}

var fontFamilies = map[string]string{
	"system":        "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
	"georgia":       "Georgia, 'Times New Roman', serif",
	"helvetica":     "'Helvetica Neue', Helvetica, Arial, sans-serif",
	"mono-system":   "ui-monospace, 'SF Mono', 'Cascadia Code', 'Segoe UI Mono', monospace",
	"mono-firacode": "'Fira Code', monospace",
	"mono-monaco":   "Monaco, 'Consolas', monospace",
	"mono-courier":  "'Courier New', Courier, monospace",
}

var fontSizes = map[string]string{
	"very-small": "0.8rem",
	"small":      "0.9rem",
	"default":    "1rem",
	"big":        "1.1rem",
	"bigger":     "1.25rem",
}

// IsDark returns true if the dark class should be applied.
// For "system", the caller must handle it client-side.
func (p UIPrefs) IsDark() string {
	switch p.Theme {
	case "dark":
		return "true"
	case "light":
		return "false"
	default:
		return "matchMedia('(prefers-color-scheme:dark)').matches"
	}
}

// UIFontFamily returns the CSS font-family for the UI font.
func (p UIPrefs) UIFontFamily() string { return fontFamilies[p.UIFont] }

// EditorFontFamily returns the CSS font-family for the editor font.
func (p UIPrefs) EditorFontFamily() string { return fontFamilies[p.EditorFont] }

// UIFontSizeCSS returns the CSS font-size for the UI.
func (p UIPrefs) UIFontSizeCSS() string { return fontSizes[p.UIFontSize] }

// EditorFontSizeCSS returns the CSS font-size for the editor.
func (p UIPrefs) EditorFontSizeCSS() string { return fontSizes[p.EditorFontSize] }

func ReadUIPrefs(r *http.Request) UIPrefs {
	p := uiPrefsDefaults
	if c, err := r.Cookie("toki-theme"); err == nil && c.Value != "" {
		p.Theme = c.Value
	}
	if c, err := r.Cookie("toki-ui-font"); err == nil && c.Value != "" {
		p.UIFont = c.Value
	}
	if c, err := r.Cookie("toki-editor-font"); err == nil && c.Value != "" {
		p.EditorFont = c.Value
	}
	if c, err := r.Cookie("toki-ui-font-size"); err == nil && c.Value != "" {
		p.UIFontSize = c.Value
	}
	if c, err := r.Cookie("toki-editor-font-size"); err == nil && c.Value != "" {
		p.EditorFontSize = c.Value
	}
	return p
}

func isValidUIPref(name, value string) bool {
	switch name {
	case "toki-theme":
		switch value {
		case "light", "dark", "system":
			return true
		}
	case "toki-ui-font", "toki-editor-font":
		_, ok := fontFamilies[value]
		return ok
	case "toki-ui-font-size", "toki-editor-font-size":
		_, ok := fontSizes[value]
		return ok
	}
	return false
}

func (p UIPrefs) Signals() settingsPrefSignals {
	return settingsPrefSignals{
		PrefTheme:          p.Theme,
		PrefUIFont:         p.UIFont,
		PrefEditorFont:     p.EditorFont,
		PrefUIFontSize:     p.UIFontSize,
		PrefEditorFontSize: p.EditorFontSize,
	}
}

func (s settingsPrefSignals) Valid() bool {
	return isValidUIPref("toki-theme", s.PrefTheme) &&
		(s.PrefThemeResolved == "light" || s.PrefThemeResolved == "dark") &&
		isValidUIPref("toki-ui-font", s.PrefUIFont) &&
		isValidUIPref("toki-editor-font", s.PrefEditorFont) &&
		isValidUIPref("toki-ui-font-size", s.PrefUIFontSize) &&
		isValidUIPref("toki-editor-font-size", s.PrefEditorFontSize)
}

func (s settingsPrefSignals) UIPrefs() UIPrefs {
	return UIPrefs{
		Theme:          s.PrefTheme,
		UIFont:         s.PrefUIFont,
		EditorFont:     s.PrefEditorFont,
		UIFontSize:     s.PrefUIFontSize,
		EditorFontSize: s.PrefEditorFontSize,
	}
}

func (s settingsPrefSignals) Normalized() settingsPrefSignals {
	switch s.PrefTheme {
	case "dark":
		s.PrefThemeResolved = "dark"
	case "light":
		s.PrefThemeResolved = "light"
	}
	return s
}

func writeUIPrefCookieScript(
	script *strings.Builder, name, value string, maxAgeSeconds int,
) {
	fmt.Fprintf(script,
		"document.cookie=%q;",
		fmt.Sprintf("%s=%s; Path=/; Max-Age=%d; SameSite=Lax",
			name, value, maxAgeSeconds))
}

func writeRootStyleScript(script *strings.Builder, name, value string) {
	fmt.Fprintf(script,
		"document.documentElement.style.setProperty(%q,%q);",
		name, value)
}

func applyUIPrefsScript(p UIPrefs) string {
	maxAgeSeconds := int((365 * 24 * time.Hour).Seconds())

	var script strings.Builder
	writeUIPrefCookieScript(&script, "toki-theme", p.Theme, maxAgeSeconds)
	writeUIPrefCookieScript(&script, "toki-ui-font", p.UIFont, maxAgeSeconds)
	writeUIPrefCookieScript(&script, "toki-editor-font", p.EditorFont, maxAgeSeconds)
	writeUIPrefCookieScript(&script, "toki-ui-font-size", p.UIFontSize, maxAgeSeconds)
	writeUIPrefCookieScript(&script, "toki-editor-font-size",
		p.EditorFontSize, maxAgeSeconds)

	script.WriteString("document.documentElement.classList.toggle('dark',")
	script.WriteString(p.IsDark())
	script.WriteString(");")
	writeRootStyleScript(&script, "--font-ui", p.UIFontFamily())
	writeRootStyleScript(&script, "--font-editor", p.EditorFontFamily())
	writeRootStyleScript(&script, "--font-size-ui", p.UIFontSizeCSS())
	writeRootStyleScript(&script, "--font-size-editor", p.EditorFontSizeCSS())

	return script.String()
}
