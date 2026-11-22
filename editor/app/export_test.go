package app

import (
	"net/http"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/internal/tokibundle"
)

// Accessors for the tests in package app_test.

var (
	DomainSignalKey        = domainSignalKey
	NormalizeDomainsSignal = normalizeDomainsSignal
)

func (a *App) ReaderFor(r *http.Request) tokibundle.Reader { return a.readerFor(r) }

func (a *App) TIKs() []*template.TIK { return a.tiks }

func (a *App) Catalogs() []*template.Catalog { return a.catalogs }

func (a *App) SetLocaleOrder(order []string) (changed bool, err error) {
	return a.setLocaleOrder(order)
}

func (a *App) NumMissing() int { return a.numMissing }

func (a *App) Changed() []*template.ICUMessage { return a.changed }

// LoadedFromIndexDB reports whether the last init took the fast path,
// which restores state without scanning the source.
func (a *App) LoadedFromIndexDB() bool { return a.scan == nil }

func (a *App) SetMessage(id, locale, message string) error {
	return a.setMessage(id, locale, message)
}

func (a *App) ResetMessage(id, locale string) (string, error) {
	return a.resetMessage(id, locale)
}

func (a *App) BuildFilteredDataIndex(
	filterType string, showLocales, showDomains map[string]bool,
	pageIdx, pageSize int, searchQuery string,
) template.DataIndex {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.buildFilteredDataIndex(
		filterType, showLocales, showDomains, pageIdx, pageSize, searchQuery,
	)
}
