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

func setUIPrefCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int((365 * 24 * time.Hour).Seconds()),
		SameSite: http.SameSiteLaxMode,
		HttpOnly: false,
	})
}

func rootStyleVarForUIPref(name string) string {
	switch name {
	case "toki-ui-font":
		return "--font-ui"
	case "toki-editor-font":
		return "--font-editor"
	case "toki-ui-font-size":
		return "--font-size-ui"
	case "toki-editor-font-size":
		return "--font-size-editor"
	default:
		return ""
	}
}

func rootStyleValueForUIPref(name, value string) string {
	switch name {
	case "toki-ui-font", "toki-editor-font":
		return fontFamilies[value]
	case "toki-ui-font-size", "toki-editor-font-size":
		return fontSizes[value]
	default:
		return ""
	}
}

func applyUIPrefScript(name, value string) string {
	maxAgeSeconds := int((365 * 24 * time.Hour).Seconds())

	var script strings.Builder
	script.WriteString(fmt.Sprintf(
		"document.cookie=%q;",
		fmt.Sprintf("%s=%s; Path=/; Max-Age=%d; SameSite=Lax", name, value, maxAgeSeconds),
	))

	switch name {
	case "toki-theme":
		darkExpr := "false"
		switch value {
		case "dark":
			darkExpr = "true"
		case "system":
			darkExpr = "matchMedia('(prefers-color-scheme: dark)').matches"
		}
		script.WriteString(
			"document.documentElement.classList.toggle('dark'," + darkExpr + ");",
		)
		script.WriteString("document.dispatchEvent(new CustomEvent('toki-theme-change'));")
	default:
		styleVar := rootStyleVarForUIPref(name)
		if styleVar == "" {
			return script.String()
		}
		styleValue := rootStyleValueForUIPref(name, value)
		if styleValue == "" {
			script.WriteString(fmt.Sprintf(
				"document.documentElement.style.removeProperty(%q);", styleVar,
			))
		} else {
			script.WriteString(fmt.Sprintf(
				"document.documentElement.style.setProperty(%q,%q);",
				styleVar, styleValue,
			))
		}
	}

	return script.String()
}
