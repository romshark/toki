package template

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/romshark/morpheus/neo"

	"github.com/romshark/toki/editor/app/datapagesgen/action"
	"github.com/romshark/toki/internal/icu"
)

// ActionExcludeEditor excludes the "editor" signal tree (and default
// "_"-prefixed signals) from POST payloads.
const ActionExcludeEditor = `(^_|\._|^editor\.)`

// NoEditorSignals applies ActionExcludeEditor to an action.
var NoEditorSignals = action.WithFilterSignals("", ActionExcludeEditor)

// LangSystem is the interface language preference that follows the
// languages the browser asks for.
const LangSystem = "system"

// codeMark stands in for a code span in a localized text, which [withCode]
// renders. Pass it as the argument of the TIK's placeholder for the code.
const codeMark = "￼"

// EditorID is the element id of the <toki-editor> editing the message of
// TIK tikID in locale.
func EditorID(tikID, locale string) string {
	return "editor-" + tikID + "-" + locale
}

// The keys of the collapsible sections. The TIKs page has the first 3 in
// its sidebar. The TIK page has the next in its sidebar and the last in its
// main area.
const (
	SectionDomains     = "domains"
	SectionLocales     = "locales"
	SectionFilter      = "filter"
	SectionMessages    = "messages"
	SectionOccurrences = "occurrences"
)

// The bounds of the width that the user drags the sidebar to. The minimum
// keeps every label of the sidebar on 1 line in every language.
const (
	SidebarMinWidth = "15rem"
	SidebarMaxWidth = "32rem"
)

// CollapsibleSections lists the keys of the collapsible sections.
var CollapsibleSections = []string{
	SectionDomains, SectionLocales, SectionFilter, SectionMessages, SectionOccurrences,
}

// sectionOpen returns the open attribute of collapsible section key. A page load
// sets it from closed. A server re-render passes nil and leaves the attribute
// unset, which keeps the section as the user set it.
func sectionOpen(closed map[string]bool, key string) neo.Attr[bool] {
	if closed == nil {
		return neo.Attr[bool]{}
	}
	return neo.Set(!closed[key])
}

// sectionTogglePayload is the payload that collapsible section key sends
// when the user opens or closes it.
func sectionTogglePayload(key string) string {
	k, _ := json.Marshal(key) // A string always marshals.
	return fmt.Sprintf("{section: %s, open: evt.detail.open}", k)
}

// tiksSectionToggle is the action that collapsible section key of the TIKs
// page calls when the user opens or closes it.
func tiksSectionToggle(key string) string {
	return action.PageTIKs.SetSectionOpen.POST(action.WithPayload(sectionTogglePayload(key)))
}

// tikSectionToggle is the action that collapsible section key of the page
// of TIK id calls when the user opens or closes it.
func tikSectionToggle(id, key string) string {
	return action.PageTIK.SetSectionOpen.POST(id, action.WithPayload(sectionTogglePayload(key)))
}

// localeOrderItemIDPrefix starts the element id of each item of a sortable
// locale list. The list reports its order as these ids.
const localeOrderItemIDPrefix = "locale-order-"

// localeOrderItemID is the element id of locale's item in a sortable
// locale list.
func localeOrderItemID(locale string) string {
	return localeOrderItemIDPrefix + locale
}

// LocaleOfOrderItemID returns the locale of the sortable locale list item
// with element id, and false if id is no such item.
func LocaleOfOrderItemID(id string) (locale string, ok bool) {
	return strings.CutPrefix(id, localeOrderItemIDPrefix)
}

// EditorValueSignal returns the JS expression $editor[tikID][locale].
// Bracket notation is required for hyphenated locales (e.g. en-US).
func EditorValueSignal(tikID, locale string) string {
	return fmt.Sprintf("$editor['%s']['%s']", tikID, locale)
}

// editorChangeBefore copies a <toki-editor> change into $icumsg, which the
// set actions read, and into $editor, which the editor's data-attr:value
// binding follows.
func editorChangeBefore(tikID, locale string) action.Option {
	return action.WithBefore(
		"$icumsg=evt.detail.value;" +
			EditorValueSignal(tikID, locale) + "=evt.detail.value",
	)
}

// InitEditorSignals returns the JSON seed for data-signals:editor.
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

// InitEditorSignalsOne is the single-TIK variant.
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
	Motion         string // "animated", "reduced", "system"
	Contrast       string // "standard", "more", "system"
	UIFont         string
	EditorFont     string
	UIFontSize     string
	EditorFontSize string
	// Pre-resolved CSS values.
	UIFontCSS         string // CSS font-family
	EditorFontCSS     string // CSS font-family
	UIFontSizeCSS     string // CSS font-size
	EditorFontSizeCSS string // CSS font-size
	SidebarWidthCSS   string // CSS width of the sidebar, "" for the default
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
	SignalKey string // FullName encoded as a Datastar signal key, an identifier.
	Name      string // Display name.
}

type DataIndex struct {
	IsHybrid bool // True when running as a native Wails desktop app.
	Dir      string
	TIKs     []TIK // windowed slice of full TIKs to render
	Catalogs []*Catalog
	Domains  []DomainFilter

	// ClosedSections holds the collapsible sections that the user closed,
	// read from the cookies on a page load. A server re-render leaves it nil,
	// which keeps each section as the user set it. See [sectionOpen].
	ClosedSections map[string]bool

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
	ID string
	// Problems lists the plural and select options the message lacks or
	// mustn't have.
	Problems        []icu.Problem
	Message         string
	Error           *ICUError // nil when the message is valid.
	MessageOriginal string
	Catalog         *Catalog
	Changed         bool
	IsReadOnly      bool
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
