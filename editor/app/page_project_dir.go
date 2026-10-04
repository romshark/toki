package app

import (
	"net/http"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app/datapagesgen/href"
	"github.com/romshark/toki/editor/app/template"
)

// PageProjectDir is /project-dir
type PageProjectDir struct {
	App *App
	PrefsSync
}

func (p PageProjectDir) GET(r *http.Request) (
	body datapages.Component,
	redirect datapages.Redirect,
	enableBackgroundStreaming datapages.EnableBackgroundStreaming,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	enableBackgroundStreaming = true
	disableRefreshAfterHidden = true

	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		redirect.URL = href.PageBuildBundle()
		return
	}

	body = p.pageLocked()
	return
}

func (p PageProjectDir) OnUpdated(event EventUpdated, sse datapages.SSE) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return sse.Redirect(href.PageBuildBundle())
	}
	return sse.PatchElement(p.pageLocked())
}

// pageLocked renders the page from the current project state.
// Caller holds the App lock.
func (p PageProjectDir) pageLocked() datapages.Component {
	return template.PageProjectDir(
		p.App.dir,
		p.App.initErr,
		p.App.repairErr,
		len(p.App.changed),
		p.App.numCorrupt,
		p.App.numMissing,
		p.App.sourceErrors,
		p.App.PickDirectory != nil,
	)
}

// POSTPick is /project-dir/pick/{$}
//
// Only meaningful in hybrid (Wails) mode: opens the native OS directory
// picker and opens the selected project. No-op in web/server mode.
func (p PageProjectDir) POSTPick(
	r *http.Request,
	updated datapages.Dispatcher[EventUpdated],
) (redirect datapages.Redirect, err error) {
	if p.App.PickDirectory == nil {
		return redirect, nil
	}
	picked, err := p.App.PickDirectory()
	if err != nil || picked == "" {
		return redirect, nil
	}
	return p.open(picked, updated)
}

// POSTOpen is /project-dir/open/{$}
//
// Opens the project from the folder path in the signal (populated either
// by the user typing in web mode, or by POSTPick's native dialog in
// hybrid mode).
func (p PageProjectDir) POSTOpen(
	r *http.Request,
	signals datapages.Signals[struct {
		Folder string `json:"folder"`
	}],
	updated datapages.Dispatcher[EventUpdated],
) (redirect datapages.Redirect, err error) {
	if signals.Values.Folder == "" {
		return datapages.Redirect{URL: href.PageProjectDir()}, nil
	}
	return p.open(signals.Values.Folder, updated)
}

// open switches the editor to the project in dir and notifies every tab.
// The client lands on the dashboard, or back on this page when the
// project can't be loaded.
func (p PageProjectDir) open(
	dir string, updated datapages.Dispatcher[EventUpdated],
) (datapages.Redirect, error) {
	target := href.PageIndex()
	if err := p.App.SetDir(dir); err != nil {
		target = href.PageProjectDir()
	}
	if err := updated.Dispatch(EventUpdated{}); err != nil {
		return datapages.Redirect{}, err
	}
	return datapages.Redirect{URL: target}, nil
}
