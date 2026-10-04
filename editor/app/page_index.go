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
	err error,
) {
	if p.App.IsLoading() {
		return template.PageLoading(), redirect, nil
	}

	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return nil, datapages.Redirect{URL: href.PageBuildBundle()}, nil
	}

	p.App.clearBuildResultLocked()

	if p.App.mustRedirectToProjectDir() {
		return nil, datapages.Redirect{URL: href.PageProjectDir()}, nil
	}

	return template.PageDashboard(p.App.buildDashboardStats()), redirect, nil
}

func (p PageIndex) OnUpdated(event EventUpdated, sse datapages.SSE) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return sse.Redirect(href.PageBuildBundle())
	}

	if p.App.mustRedirectToProjectDir() {
		return sse.Redirect(href.PageProjectDir())
	}

	return sse.PatchElement(template.PageDashboard(p.App.buildDashboardStats()))
}
