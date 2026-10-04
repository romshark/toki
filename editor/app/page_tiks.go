package app

import (
	"net/http"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app/datapagesgen/href"
	"github.com/romshark/toki/editor/app/template"
)

// PageTIKs is /tiks
type PageTIKs struct {
	App *App
	PrefsSync
	ResetSync
}

// StateTIKs is the per-tab view state of PageTIKs: the filters and the
// page the tab shows. StreamOpen seeds it from the URL-synced signals.
type StateTIKs struct {
	FilterType  string
	ShowLocales map[string]bool // nil shows all
	ShowDomains map[string]bool // nil shows all
	SearchQuery string
	PageIdx     int // 0-based
	PageSize    int
}

func (p PageTIKs) GET(
	r *http.Request,
	query datapages.Query[struct {
		Filter   string `query:"f" reflectsignal:"filtertype"`
		Locales  string `query:"l" reflectsignal:"shownlocales"`
		Domains  string `query:"d" reflectsignal:"showndomains"`
		Search   string `query:"q" reflectsignal:"searchquery"`
		Page     int    `query:"p" reflectsignal:"page"`
		PageSize int    `query:"ps" reflectsignal:"pagesize"`
	}],
) (
	body datapages.Component,
	redirect datapages.Redirect,
	enableBackgroundStreaming datapages.EnableBackgroundStreaming,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	enableBackgroundStreaming = true
	disableRefreshAfterHidden = true

	if p.App.IsLoading() {
		body = template.PageLoading()
		return
	}

	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		redirect.URL = href.PageBuildBundle()
		return
	}

	p.App.clearBuildResultLocked()

	if p.App.mustRedirectToProjectDir() {
		redirect.URL = href.PageProjectDir()
		return
	}

	q := query.Values
	data := p.App.buildFilteredDataIndex(
		q.Filter, parseLocalesParam(q.Locales), parseDomainsParam(q.Domains),
		// URL is 1-based, internal is 0-based
		max(q.Page-1, 0),
		q.PageSize, q.Search)
	body = template.PageTIKs(data)
	return
}

func (PageTIKs) StreamOpen(
	r *http.Request,
	state datapages.State[StateTIKs],
	signals datapages.Signals[struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		Page        int             `json:"page"`
		PageSize    int             `json:"pagesize"`
	}],
) error {
	s := signals.Values
	*state.Values = StateTIKs{
		FilterType:  normalizeFilterType(s.FilterType),
		ShowLocales: s.ShowLocales,
		ShowDomains: normalizeDomainsSignal(s.ShowDomains),
		SearchQuery: s.SearchQuery,
		// signal is 1-based, internal is 0-based
		PageIdx:  max(s.Page-1, 0),
		PageSize: template.NormalizePageSize(s.PageSize),
	}
	return nil
}

func (p PageTIKs) OnUpdated(
	event EventUpdated,
	sse datapages.SSE,
	state datapages.State[StateTIKs],
	stateID string,
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return sse.Redirect(href.PageBuildBundle())
	}

	if p.App.mustRedirectToProjectDir() {
		return sse.Redirect(href.PageProjectDir())
	}

	// If this tab triggered the change, leave the changed editor's signal
	// alone so that in-progress typing is never overwritten by its own
	// stale echo.
	var exclude string
	if event.SourceStateID == stateID {
		exclude = event.ChangedEditor
	}
	return p.renderLocked(sse, state.Values, exclude)
}

// tiksSignals are the signals the server owns on the TIKs page. It patches
// them before every render so the URL, the pagination and the editors
// follow the view state.
type tiksSignals struct {
	ShowLocales  map[string]bool              `json:"showlocales"`
	ShowDomains  map[string]bool              `json:"showdomains"`
	ShownLocales string                       `json:"shownlocales"`
	ShownDomains string                       `json:"showndomains"`
	Page         int                          `json:"page"`
	PageSize     int                          `json:"pagesize"`
	Editor       map[string]map[string]string `json:"editor"`
}

// renderLocked patches the whole TIKs page rendered from the tab's view
// state. excludeEditor names the editor whose signal is left alone
// (see [editorSignalsFor]). Caller holds the App lock.
func (p PageTIKs) renderLocked(
	sse datapages.SSE, vs *StateTIKs, excludeEditor string,
) error {
	data := p.App.buildFilteredDataIndex(
		vs.FilterType, vs.ShowLocales, vs.ShowDomains,
		vs.PageIdx, vs.PageSize, vs.SearchQuery)
	// The build may have clamped the page index or normalized the page
	// size, keep the state in sync.
	vs.PageIdx = data.PageIdx
	vs.PageSize = data.PageSize

	localeSignals := make(map[string]bool, len(data.Catalogs))
	for _, c := range data.Catalogs {
		localeSignals[c.Locale] = vs.ShowLocales == nil || vs.ShowLocales[c.Locale]
	}
	domainSignals := make(map[string]bool, len(data.Domains))
	for _, d := range data.Domains {
		domainSignals[d.SignalKey] = vs.ShowDomains == nil || vs.ShowDomains[d.FullName]
	}

	// Signals first: the morph's data-attr:value bindings read them.
	if err := sse.PatchSignals(tiksSignals{
		ShowLocales:  localeSignals,
		ShowDomains:  domainSignals,
		ShownLocales: serializeShownSignal(vs.ShowLocales),
		ShownDomains: serializeShownSignal(vs.ShowDomains),
		Page:         data.PageIdx + 1,
		PageSize:     data.PageSize,
		Editor:       editorSignalsFor(data.TIKs, excludeEditor),
	}); err != nil {
		return err
	}
	return sse.PatchElement(template.PageTIKs(data))
}

// render is renderLocked for actions, which don't hold the App lock.
func (p PageTIKs) render(sse datapages.SSE, vs *StateTIKs) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.mustRedirectToProjectDir() {
		return datapages.ErrBadRequest
	}
	return p.renderLocked(sse, vs, "")
}

// scrollToTop scrolls the list back to its start after a page change.
// The scroll position is out of the server's reach, hence the script.
func scrollToTop(sse datapages.SSE) error {
	return sse.ExecuteScript(
		`document.querySelector('#page-tiks main')?.scrollTo({top:0})`,
	)
}

// POSTSet is /tiks/set/{$}
func (p PageTIKs) POSTSet(
	r *http.Request,
	query datapages.Query[struct {
		TIKID  string `query:"t"`
		Locale string `query:"l"`
	}],
	signals datapages.Signals[struct {
		ICUMsg string `json:"icumsg"`
	}],
	_ datapages.State[StateTIKs],
	stateID string,
	updated datapages.Dispatcher[EventUpdated],
) error {
	return p.App.setMessageFromTab(
		query.Values.TIKID, query.Values.Locale, signals.Values.ICUMsg,
		stateID, updated,
	)
}

// POSTFilter is /tiks/filter/{$}
func (p PageTIKs) POSTFilter(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateTIKs],
	signals datapages.Signals[struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
	}],
) error {
	vs, s := state.Values, signals.Values
	ft := normalizeFilterType(s.FilterType)
	if vs.FilterType != ft || vs.SearchQuery != s.SearchQuery {
		vs.PageIdx = 0
	}
	vs.FilterType = ft
	vs.ShowLocales = s.ShowLocales
	vs.ShowDomains = normalizeDomainsSignal(s.ShowDomains)
	vs.SearchQuery = s.SearchQuery
	return p.render(sse, vs)
}

// POSTShowAllLocales is /tiks/show-all-locales/{$}
func (p PageTIKs) POSTShowAllLocales(
	r *http.Request, sse datapages.SSE, state datapages.State[StateTIKs],
) error {
	state.Values.ShowLocales = nil
	return p.render(sse, state.Values)
}

// POSTHideAllLocales is /tiks/hide-all-locales/{$}
func (p PageTIKs) POSTHideAllLocales(
	r *http.Request, sse datapages.SSE, state datapages.State[StateTIKs],
) error {
	state.Values.ShowLocales = map[string]bool{}
	return p.render(sse, state.Values)
}

// POSTShowAllDomains is /tiks/show-all-domains/{$}
func (p PageTIKs) POSTShowAllDomains(
	r *http.Request, sse datapages.SSE, state datapages.State[StateTIKs],
) error {
	state.Values.ShowDomains = nil
	return p.render(sse, state.Values)
}

// POSTHideAllDomains is /tiks/hide-all-domains/{$}
func (p PageTIKs) POSTHideAllDomains(
	r *http.Request, sse datapages.SSE, state datapages.State[StateTIKs],
) error {
	state.Values.ShowDomains = map[string]bool{}
	return p.render(sse, state.Values)
}

// POSTSetPage is /tiks/set-page/{$}
func (p PageTIKs) POSTSetPage(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateTIKs],
	signals datapages.Signals[struct {
		Page int `json:"page"`
	}],
) error {
	// signal is 1-based, internal is 0-based
	state.Values.PageIdx = max(signals.Values.Page-1, 0)
	if err := scrollToTop(sse); err != nil {
		return err
	}
	return p.render(sse, state.Values)
}

// POSTSetPageSize is /tiks/set-page-size/{$}
//
// Changing the per-page count preserves the user's position by mapping
// the first item of the old page onto the new page that contains it.
func (p PageTIKs) POSTSetPageSize(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[StateTIKs],
	signals datapages.Signals[struct {
		PageSize int `json:"pagesize"`
	}],
) error {
	vs := state.Values
	newSize := template.NormalizePageSize(signals.Values.PageSize)
	if vs.PageSize > 0 && newSize != vs.PageSize {
		firstItem := vs.PageIdx * vs.PageSize
		vs.PageIdx = firstItem / newSize
	}
	vs.PageSize = newSize
	if err := scrollToTop(sse); err != nil {
		return err
	}
	return p.render(sse, vs)
}
