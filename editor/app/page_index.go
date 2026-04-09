package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
)

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	r *http.Request,
) (
	body templ.Component,
	redirect string,
	err error,
) {
	if p.App.IsLoading() {
		return template.PageLoading(), "", nil
	}

	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return nil, href.PageBuildBundle(), nil
	}

	p.App.clearBuildResultLocked()

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		return nil, href.PageProjectDir(), nil
	}

	stats := p.App.buildDashboardStats()
	return template.PageDashboard(stats), "", nil
}

func (p PageIndex) OnUpdated(
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

	stats := p.App.buildDashboardStats()
	return sse.PatchElementTempl(template.PageDashboard(stats))
}
