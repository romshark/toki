package app

import (
	"net/http"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app/datapagesgen/href"
	"github.com/romshark/toki/editor/app/template"
)

// PageIndex is /
type PageIndex struct {
	App *App
	PrefsSync
}

func (p PageIndex) GET(
	r *http.Request,
) (
	body datapages.Component,
	redirect datapages.Redirect,
	docAttrs datapages.DocumentAttrs,
	err error,
) {
	tr := p.App.readerFor(r)
	docAttrs = documentAttrs(tr)

	if p.App.IsLoading() {
		return template.PageLoading(tr), redirect, docAttrs, nil
	}

	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return nil, datapages.Redirect{URL: href.PageBuildBundle()}, nil, nil
	}

	p.App.clearBuildResultLocked()

	if p.App.mustRedirectToProjectDir() {
		return nil, datapages.Redirect{URL: href.PageProjectDir()}, nil, nil
	}

	return template.PageDashboard(tr, p.App.buildDashboardStats()), redirect, docAttrs, nil
}

func (p PageIndex) StreamOpen(r *http.Request, state datapages.State[StateLang]) error {
	state.Values.Reader = p.App.readerFor(r)
	return nil
}

func (p PageIndex) OnUpdated(
	event EventUpdated, sse datapages.SSE, state datapages.State[StateLang],
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return sse.Redirect(href.PageBuildBundle())
	}

	if p.App.mustRedirectToProjectDir() {
		return sse.Redirect(href.PageProjectDir())
	}

	return sse.PatchElement(
		template.PageDashboard(state.Values.Reader, p.App.buildDashboardStats()),
	)
}
