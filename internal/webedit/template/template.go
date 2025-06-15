package template

import (
	"iter"
	"net/http"

	"github.com/romshark/toki/internal/log"
)

type FilterTIKs int8

const (
	FilterTIKsAll FilterTIKs = iota
	FilterTIKsEmpty
	FilterTIKsComplete
	FilterTIKsIncomplete
	FilterTIKsInvalid
)

type ICUMessage struct {
	Locale            string
	Message           string
	Error             string
	IncompleteReports []string
}

type TIK struct {
	ID          string
	TIK         string
	Description string
	ICU         []ICUMessage
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
	NumChanges        int
	NumAll            int
	NumEmpty          int
	NumComplete       int
	NumIncomplete     int
	NumInvalid        int
}

func RenderPageIndex(w http.ResponseWriter, r *http.Request, data DataIndex) {
	if err := viewIndex(
		data.TIKs,
		data.Catalogs,
		data.CatalogsDisplayed,
		data.FilterTIKs,
		data.NumChanges,
		data.NumAll,
		data.NumEmpty,
		data.NumComplete,
		data.NumIncomplete,
		data.NumInvalid,
	).Render(r.Context(), w); err != nil {
		log.Error("rendering page index", err)
	}
}

func RenderViewIndex(w http.ResponseWriter, r *http.Request, data DataIndex) {
	if err := pageIndex(
		data.TIKs,
		data.Catalogs,
		data.CatalogsDisplayed,
		data.FilterTIKs,
		data.NumChanges,
		data.NumAll,
		data.NumEmpty,
		data.NumComplete,
		data.NumIncomplete,
		data.NumInvalid,
	).Render(r.Context(), w); err != nil {
		log.Error("rendering page index", err)
	}
}

func isCatalogDefault(catalogs []*Catalog, locale string) bool {
	for _, c := range catalogs {
		if c.Locale == locale {
			return c.Default
		}
	}
	return false
}
