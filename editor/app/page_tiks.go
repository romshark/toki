package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
	"github.com/romshark/toki/editor/datapagesgen/httperr"
)

// PageTIKs is /tiks
type PageTIKs struct{ App *App }

func (p PageTIKs) GET(
	r *http.Request,
	query struct {
		Filter  string `query:"f" reflectsignal:"filtertype"`
		Locales string `query:"l" reflectsignal:"shownlocales"`
		Domains string `query:"d" reflectsignal:"showndomains"`
		Search  string `query:"q" reflectsignal:"searchquery"`
	},
) (
	body templ.Component,
	redirect string,
	enableBackgroundStreaming bool,
	disableRefreshAfterHidden bool,
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
		redirect = href.PageBuildBundle()
		return
	}

	p.App.clearBuildResultLocked()

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		redirect = href.PageProjectDir()
		return
	}

	showLocales := parseLocalesParam(query.Locales)
	showDomains := parseDomainsParam(query.Domains)
	data := p.App.buildFilteredDataIndex(
		query.Filter, showLocales, showDomains, 0, query.Search)
	body = template.PageTIKs(data)
	return
}

func (p PageTIKs) StreamOpen(
	r *http.Request,
	streamID uint64,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()
	p.App.registerTIKsStreamLocked(streamID, signals.InstanceID, pageTIKsState{
		filterType:  normalizeFilterType(signals.FilterType),
		showLocales: signals.ShowLocales,
		showDomains: normalizeDomainsSignal(signals.ShowDomains),
		searchQuery: signals.SearchQuery,
	})
	return nil
}

func (p PageTIKs) StreamClose(r *http.Request, streamID uint64) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()
	p.App.unregisterTIKsStreamLocked(streamID)
	return nil
}

func (p PageTIKs) OnUpdated(
	event EventUpdated,
	sse *datastar.ServerSentEventGenerator,
	streamID uint64,
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return sse.ExecuteScript(navigate(href.PageBuildBundle()))
	}

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		return nil
	}

	instID := p.App.streamInst[streamID]
	vs := p.App.tiksViews[instID]
	if vs == nil {
		return nil
	}

	// If this SSE connection belongs to the tab that triggered the
	// change, exclude the changed editor from the sync so that in-
	// progress typing is never overwritten by its own stale echo.
	var exclude string
	if event.SourceInstanceID != "" && event.SourceInstanceID == instID {
		exclude = event.ChangedEditor
	}

	data := p.App.buildFilteredDataIndex(
		vs.filterType, vs.showLocales, vs.showDomains, vs.windowStart, vs.searchQuery)
	if err := sse.PatchElementTempl(template.PageTIKsContent(data)); err != nil {
		return err
	}
	return sse.ExecuteScript(syncEditorsScript(data.TIKs, exclude))
}

func (PageTIKs) OnReset(
	event EventReset,
	sse *datastar.ServerSentEventGenerator,
) error {
	if event.ResetEditor == "" {
		return nil
	}
	if err := sse.MarshalAndPatchSignals(struct {
		ResetDoneTIKID  string `json:"resetdonetikid"`
		ResetDoneLocale string `json:"resetdonelocale"`
	}{
		ResetDoneTIKID:  event.TIKID,
		ResetDoneLocale: event.Locale,
	}); err != nil {
		return err
	}
	return sse.ExecuteScript(fmt.Sprintf(
		"resetEditorValue(%q,%q)", event.ResetEditor, event.ResetValue,
	))
}

// renderFromViewState renders the TIKs page from the current server-side
// view state and pushes updated signals for checkbox bindings and URL sync.
func (p PageTIKs) renderFromViewState(
	sse *datastar.ServerSentEventGenerator, vs *pageTIKsState,
) error {
	data := p.App.buildFilteredDataIndex(
		vs.filterType, vs.showLocales, vs.showDomains,
		vs.windowStart, vs.searchQuery)
	if err := sse.PatchElementTempl(template.PageTIKsContent(data)); err != nil {
		return err
	}

	// Push checkbox signal state so data-bind stays in sync.
	localeSignals := make(map[string]bool, len(data.Catalogs))
	for _, c := range data.Catalogs {
		localeSignals[c.Locale] = vs.showLocales == nil || vs.showLocales[c.Locale]
	}
	domainSignals := make(map[string]bool, len(data.Domains))
	for _, d := range data.Domains {
		domainSignals[d.SignalKey] = vs.showDomains == nil || vs.showDomains[d.FullName]
	}

	if err := sse.MarshalAndPatchSignals(struct {
		ShowLocales  map[string]bool `json:"showlocales"`
		ShowDomains  map[string]bool `json:"showdomains"`
		ShownLocales string          `json:"shownlocales"`
		ShownDomains string          `json:"showndomains"`
	}{
		ShowLocales:  localeSignals,
		ShowDomains:  domainSignals,
		ShownLocales: serializeShownSignal(vs.showLocales),
		ShownDomains: serializeShownSignal(vs.showDomains),
	}); err != nil {
		return err
	}
	return sse.ExecuteScript(syncEditorsScript(data.TIKs, ""))
}

// POSTFilter is /tiks/filter/{$}
func (p PageTIKs) POSTFilter(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		return httperr.BadRequest
	}

	vs := p.App.tiksViews[signals.InstanceID]
	if vs == nil {
		return httperr.BadRequest
	}

	ft := normalizeFilterType(signals.FilterType)
	if vs.filterType != ft || vs.searchQuery != signals.SearchQuery {
		vs.windowStart = 0
	}
	vs.filterType = ft
	vs.showLocales = signals.ShowLocales
	vs.showDomains = normalizeDomainsSignal(signals.ShowDomains)
	vs.searchQuery = signals.SearchQuery

	return p.renderFromViewState(sse, vs)
}

// POSTShowAllLocales is /tiks/show-all-locales/{$}
func (p PageTIKs) POSTShowAllLocales(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		InstanceID string `json:"instance_id"`
	},
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	vs := p.App.tiksViews[signals.InstanceID]
	if vs == nil {
		return httperr.BadRequest
	}

	vs.showLocales = nil
	return p.renderFromViewState(sse, vs)
}

// POSTHideAllLocales is /tiks/hide-all-locales/{$}
func (p PageTIKs) POSTHideAllLocales(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		InstanceID string `json:"instance_id"`
	},
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	vs := p.App.tiksViews[signals.InstanceID]
	if vs == nil {
		return httperr.BadRequest
	}

	vs.showLocales = map[string]bool{}
	return p.renderFromViewState(sse, vs)
}

// POSTShowAllDomains is /tiks/show-all-domains/{$}
func (p PageTIKs) POSTShowAllDomains(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		InstanceID string `json:"instance_id"`
	},
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	vs := p.App.tiksViews[signals.InstanceID]
	if vs == nil {
		return httperr.BadRequest
	}

	vs.showDomains = nil
	return p.renderFromViewState(sse, vs)
}

// POSTHideAllDomains is /tiks/hide-all-domains/{$}
func (p PageTIKs) POSTHideAllDomains(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		InstanceID string `json:"instance_id"`
	},
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	vs := p.App.tiksViews[signals.InstanceID]
	if vs == nil {
		return httperr.BadRequest
	}

	vs.showDomains = map[string]bool{}
	return p.renderFromViewState(sse, vs)
}

// POSTScrollDown is /tiks/scroll-down/{$}
func (p PageTIKs) POSTScrollDown(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		WindowStart int             `json:"windowstart"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	return p.handleScroll(
		sse, signals.FilterType, signals.ShowLocales,
		normalizeDomainsSignal(signals.ShowDomains),
		signals.SearchQuery, signals.WindowStart, signals.InstanceID, false)
}

// POSTScrollUp is /tiks/scroll-up/{$}
func (p PageTIKs) POSTScrollUp(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		WindowStart int             `json:"windowstart"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	return p.handleScroll(sse, signals.FilterType, signals.ShowLocales,
		normalizeDomainsSignal(signals.ShowDomains),
		signals.SearchQuery, signals.WindowStart, signals.InstanceID, true)
}

func (p PageTIKs) handleScroll(
	sse *datastar.ServerSentEventGenerator,
	filterType string, showLocales, showDomains map[string]bool,
	searchQuery string, windowStart int, instanceID string,
	scrollUp bool,
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		return httperr.BadRequest
	}

	ft := normalizeFilterType(filterType)
	if vs := p.App.tiksViews[instanceID]; vs != nil {
		vs.windowStart = windowStart
	}

	data := p.App.buildFilteredDataIndex(
		ft, showLocales, showDomains, windowStart, searchQuery,
	)

	if scrollUp {
		// Before morph: find the first visible card and record its viewport
		// position so we can restore it after items are inserted above.
		if err := sse.ExecuteScript(
			`(function(){
				var m=document.querySelector('#page-tiks main');
				var cards=m.querySelectorAll('section.card[id]');
				for(var i=0;i<cards.length;i++){
				var r=cards[i].getBoundingClientRect();
				if(r.bottom>0){window._tikAnchor={id:cards[i].id,top:r.top};return}}
			})()`,
		); err != nil {
			return err
		}
	}

	if err := sse.PatchElementTempl(template.TiksContentList(data)); err != nil {
		return err
	}

	if scrollUp {
		// After morph: find the same card and scroll so it's at the
		// same viewport position as before.
		return sse.ExecuteScript(
			`if(window._tikAnchor&&window._tikAnchor.id){
				var el=document.getElementById(window._tikAnchor.id);
				if(el){
				var m=document.querySelector('#page-tiks main');
				m.scrollTop+=el.getBoundingClientRect().top-window._tikAnchor.top
				}}
			`,
		)
	}
	return nil
}
