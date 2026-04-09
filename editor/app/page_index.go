package app

import (
	"net/http"

	"github.com/a-h/templ"
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

	p.App.mu.Lock()
	defer p.App.mu.Unlock()

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
