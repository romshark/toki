package template

// DashboardStats holds statistics for the dashboard page.
type DashboardStats struct {
	Dir         string
	NumTIKs     int
	NumLocales      int
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

type DataIndex struct {
	Dir      string
	TIKs     []TIK // windowed slice of full TIKs to render
	Catalogs []*Catalog

	// ShownLocales which locales are shown (nil = all)
	ShownLocales map[string]bool

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

type TIK struct {
	ID          string
	TIK         string
	Description string
	ICU         []*ICUMessage
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

