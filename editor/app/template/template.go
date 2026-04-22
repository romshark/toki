package template

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/romshark/toki/editor/datapagesgen/action"
)

// ActionExcludeEditor is the regex used with action.WithFilterSignals to
// exclude per-editor value signals (under the "editor" namespace) from
// POST requests. Also preserves the default "_"-prefix exclusion so
// signals like $_history.canback stay local to the client.
const ActionExcludeEditor = `(^_|\._|^editor\.)`

// NoEditorSignals is the action option that skips per-editor value
// signals when making POST requests. Pass it to any action.POSTXxx(...)
// call that doesn't need editor values — everything except the server
// side of POST /set reads the truth from App state, not from signals.
var NoEditorSignals = action.WithFilterSignals("", ActionExcludeEditor)

// EditorValueSignal returns the Datastar JS expression for the signal
// that holds the current ICU message for a given TIK/locale. Bracket
// notation is used so locales with hyphens (e.g. en-US) work.
func EditorValueSignal(tikID, locale string) string {
	return fmt.Sprintf("$editor['%s']['%s']", tikID, locale)
}

// InitEditorSignals returns a JSON literal suitable for seeding the
// top-level "editor" signal tree via data-signals:editor. Callers
// scope this to the editors visible on the page.
func InitEditorSignals(tiks []TIK) string {
	m := make(map[string]map[string]string, len(tiks))
	for i := range tiks {
		inner := make(map[string]string, len(tiks[i].ICU))
		for _, msg := range tiks[i].ICU {
			inner[msg.Catalog.Locale] = msg.Message
		}
		m[tiks[i].ID] = inner
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Errorf("unexpected JSON marshaling err: %w", err))
	}
	return string(b)
}

// InitEditorSignalsOne is the single-TIK variant used by PageTIK.
func InitEditorSignalsOne(tk *TIK) string {
	inner := make(map[string]string, len(tk.ICU))
	for _, msg := range tk.ICU {
		inner[msg.Catalog.Locale] = msg.Message
	}
	b, err := json.Marshal(map[string]map[string]string{tk.ID: inner})
	if err != nil {
		panic(fmt.Errorf("unexpected JSON marshaling err: %w", err))
	}
	return string(b)
}

// UIPrefs holds user interface appearance preferences read from cookies.
type UIPrefs struct {
	Theme          string // "light", "dark", "system"
	UIFont         string
	EditorFont     string
	UIFontSize     string
	EditorFontSize string
	// Pre-resolved CSS values.
	UIFontCSS         string // CSS font-family
	EditorFontCSS     string // CSS font-family
	UIFontSizeCSS     string // CSS font-size
	EditorFontSizeCSS string // CSS font-size
}

// DashboardStats holds statistics for the dashboard page.
type DashboardStats struct {
	IsHybrid        bool // True when running as a native Wails desktop app.
	Dir             string
	NumTIKs         int
	NumLocales      int
	NumDomains      int
	NativeLocale    LocaleStats
	Locales         []LocaleStats
	Domains         []DomainInfo
	TotalChanges    int
	CanApplyChanges bool
	NumComplete     int
	NumIncomplete   int
	NumUntranslated int
	NumInvalid      int
	Completeness    float64 // 0.0–1.0
}

// LocaleStats holds per-locale statistics for the dashboard.
type LocaleStats struct {
	Locale       string
	Name         string // e.g. "German", "English"
	Default      bool
	Complete     int     // TIKs fully translated and valid for this locale
	Incomplete   int     // TIKs whose translation is missing required ICU options
	Untranslated int     // TIKs with no translation for this locale
	Invalid      int     // TIKs with ICU errors for this locale
	Changed      int     // messages with unsaved edits
	Completeness float64 // 0.0–1.0
}

// DomainFilter holds a domain name for use in the sidebar filter UI.
type DomainFilter struct {
	FullName  string // Dot-separated qualified name used as filter value.
	SignalKey string // FullName with dots replaced by underscores (safe for Datastar signals).
	Name      string // Display name.
}

type DataIndex struct {
	IsHybrid bool // True when running as a native Wails desktop app.
	Dir      string
	TIKs     []TIK // windowed slice of full TIKs to render
	Catalogs []*Catalog
	Domains  []DomainFilter

	// ShownLocales which locales are shown (nil = all)
	ShownLocales map[string]bool

	// ShownDomains which domains are shown (nil = all)
	ShownDomains map[string]bool

	// FilterType is "all", "changed", etc.
	FilterType string

	// Pagination.
	PageIdx       int // 0-based index of the current page
	PageSize      int // number of TIKs per page
	TotalFiltered int // total TIKs after filtering

	NumAll          int
	NumChanged      int
	NumUntranslated int
	NumComplete     int
	NumIncomplete   int
	NumInvalid      int
	TotalChanges    int
	CanApplyChanges bool
	SearchQuery     string
}

// PageSizeOptions lists the page size choices offered by the
// per-page selector. The first entry is the default.
var PageSizeOptions = []int{10, 25, 50, 100}

// DefaultPageSize is the page size used when none is specified or
// when an invalid value is supplied.
const DefaultPageSize = 25

// NormalizePageSize clamps n to one of PageSizeOptions, returning
// DefaultPageSize when n is not one of the allowed values.
func NormalizePageSize(n int) int {
	if slices.Contains(PageSizeOptions, n) {
		return n
	}
	return DefaultPageSize
}

// TotalPages returns the total number of pages for the current filter.
func (d DataIndex) TotalPages() int {
	if d.PageSize <= 0 || d.TotalFiltered <= 0 {
		return 0
	}
	return (d.TotalFiltered + d.PageSize - 1) / d.PageSize
}

type ICUMessage struct {
	ID                string
	IncompleteReports []string
	Message           string
	Error             string
	MessageOriginal   string
	Catalog           *Catalog
	Changed           bool
	IsReadOnly        bool
}

// SourceOccurrence is a location where a TIK appears in the source code.
type SourceOccurrence struct {
	File        string // Absolute path.
	DisplayFile string // Path relative to project dir (for display).
	Line        int
	Column      int
}

// Pos returns ":line:col" as a single string.
func (o SourceOccurrence) Pos() string {
	return fmt.Sprintf(":%d:%d", o.Line, o.Column)
}

type TIK struct {
	ID          string
	TIK         string
	Description string
	Domain      string // Fully qualified domain name (empty if no domain).
	ICU         []*ICUMessage
	Occurrences []SourceOccurrence
	// Status flags for client-side filtering.
	IsChanged      bool
	IsUntranslated bool
	IsComplete     bool
	IsIncomplete   bool
	IsInvalid      bool
}

type Catalog struct {
	Locale  string
	Name    string // e.g. "German", "English"
	Default bool
}

// SourceError is a source-code parse error surfaced on the project-dir
// page when the bundle cannot be loaded.
type SourceError struct {
	File string // Path to the offending file (typically absolute).
	Line int
	Col  int
	Err  string
}

// BuildBundleState holds the current state for the build-bundle page.
type BuildBundleState struct {
	Building     bool
	Err          string
	Duration     time.Duration
	TotalChanges int
}

// FmtDuration formats a duration for display.
func FmtDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// DomainInfo holds display data for a single TIK domain.
type DomainInfo struct {
	Name            string
	Description     string
	Dir             string // Absolute path.
	FullName        string // Dot-separated path (e.g. "myapp.storefront.checkout").
	ParentName      string // Display name of parent domain (empty if root).
	ParentFullName  string // FullName of parent domain (empty if root).
	NumTIKs         int    // TIKs directly in this domain.
	NumComplete     int
	NumIncomplete   int
	NumUntranslated int
	NumInvalid      int
	NumChanged      int
	Completeness    float64 // 0.0–1.0
	SubDomains      []DomainInfo
}

// DataDomains holds data for the /domains/ page.
type DataDomains struct {
	Dir             string
	Domains         []DomainInfo
	TotalDomains    int
	TotalChanges    int
	CanApplyChanges bool
}
