package template

import (
	"fmt"
	"iter"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/romshark/toki/internal/log"
)

type FilterTIKs int8

func (f *FilterTIKs) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case `"all"`, `""`:
		*f = FilterTIKsAll
	case "changed":
		*f = FilterTIKsChanged
	case "empty":
		*f = FilterTIKsEmpty
	case "complete":
		*f = FilterTIKsComplete
	case "incomplete":
		*f = FilterTIKsIncomplete
	case "invalid":
		*f = FilterTIKsInvalid
	default:
		return fmt.Errorf("unexpected value: %s", string(b))
	}
	return nil
}

const (
	FilterTIKsAll FilterTIKs = iota
	FilterTIKsChanged
	FilterTIKsEmpty
	FilterTIKsComplete
	FilterTIKsIncomplete
	FilterTIKsInvalid
)

type ICUMessage struct {
	ID                string
	IncompleteReports []string
	Message           string
	Error             string

	// MessageOriginal holds the original value before the change.
	// This is only relevant when Changed == true.
	MessageOriginal string

	Catalog *Catalog

	// Changed is true when there was a change to this message.
	// In that case MessageOriginal holds the original message value.
	Changed bool

	IsReadOnly bool
}

type TIK struct {
	ID          string
	TIK         string
	Description string
	ICU         []*ICUMessage
}

type Catalog struct {
	Locale  string
	Default bool
}

type DataTIK struct {
	TIK *TIK
}

type DataIndex struct {
	TIKs              iter.Seq[TIK]
	Catalogs          []*Catalog
	CatalogsDisplayed []*Catalog
	FilterTIKs        FilterTIKs
	NumAll            int
	NumChanged        int
	NumEmpty          int
	NumComplete       int
	NumIncomplete     int
	NumInvalid        int
	TotalChanges      int
	CanApplyChanges   bool
}

func render(w http.ResponseWriter, r *http.Request, c templ.Component, name string) {
	if err := c.Render(r.Context(), w); err != nil {
		log.Error("rendering templ component", err, slog.String("component", name))
	}
}

func RenderPageIndex(w http.ResponseWriter, r *http.Request, data DataIndex) {
	render(w, r, pageIndex(data), "ViewIndex")
}

func RenderPageTIK(w http.ResponseWriter, r *http.Request, data DataTIK) {
	render(w, r, pageTIK(data), "PageTIK")
}
