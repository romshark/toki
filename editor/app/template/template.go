package template

import (
	"fmt"
	"time"
)

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
	Dir             string
	NumTIKs         int
	NumLocales      int
	NumDomains      int
	NativeLocale    LocaleStats
	Locales         []LocaleStats
	TotalChanges    int
	CanApplyChanges bool
	NumComplete     int
	NumIncomplete   int
	NumEmpty        int
	NumInvalid      int
	Completeness    float64 // 0.0–1.0
}

// LocaleStats holds per-locale statistics for the dashboard.
type LocaleStats struct {
	Locale       string
	Name         string // e.g. "German", "English"
	Default      bool
	Complete     int     // TIKs fully translated and valid for this locale
	Incomplete   int     // TIKs with empty message for this locale
	Empty        int     // TIKs with empty message for this locale
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

	// Virtual scroll window.
	WindowStart   int // index of first TIK in the window
	WindowSize    int // number of TIKs in the window
	TotalFiltered int // total TIKs after filtering

	NumAll          int
	NumChanged      int
	NumEmpty        int
	NumComplete     int
	NumIncomplete   int
	NumInvalid      int
	TotalChanges    int
	CanApplyChanges bool
	SearchQuery     string
}

const DefaultWindowSize = 30

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
	IsChanged    bool
	IsEmpty      bool
	IsComplete   bool
	IsIncomplete bool
	IsInvalid    bool
}

type Catalog struct {
	Locale  string
	Name    string // e.g. "German", "English"
	Default bool
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
	Name           string
	Description    string
	Dir            string // Absolute path.
	FullName       string // Dot-separated path (e.g. "myapp.storefront.checkout").
	ParentName     string // Display name of parent domain (empty if root).
	ParentFullName string // FullName of parent domain (empty if root).
	NumTIKs        int    // TIKs directly in this domain.
	NumComplete    int
	NumIncomplete  int
	NumEmpty       int
	NumInvalid     int
	NumChanged     int
	Completeness   float64 // 0.0–1.0
	SubDomains     []DomainInfo
}

// DataDomains holds data for the /domains/ page.
type DataDomains struct {
	Dir             string
	Domains         []DomainInfo
	TotalDomains    int
	TotalChanges    int
	CanApplyChanges bool
}
