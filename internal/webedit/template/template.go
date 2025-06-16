package template

import (
	"iter"
	"net/http"

	"github.com/romshark/toki/internal/log"
)

type FilterTIKs int8

const (
	FilterTIKsAll FilterTIKs = iota
	FilterTIKsChanged
	FilterTIKsEmpty
	FilterTIKsComplete
	FilterTIKsIncomplete
	FilterTIKsInvalid
)

type ICUMessage struct {
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

func RenderPageIndex(w http.ResponseWriter, r *http.Request, data DataIndex) {
	if err := viewIndex(data).Render(r.Context(), w); err != nil {
		log.Error("rendering page index", err)
	}
}

func RenderViewIndex(w http.ResponseWriter, r *http.Request, data DataIndex) {
	if err := viewIndex(data).Render(r.Context(), w); err != nil {
		log.Error("rendering view index", err)
	}
}

func RenderFragmentICUMessage(
	w http.ResponseWriter, r *http.Request,
	tikID string, msg *ICUMessage,
) {
	c := fragmentICUMessage(tikID, msg)
	if err := c.Render(r.Context(), w); err != nil {
		log.Error("rendering fragment icu message", err)
	}
}
