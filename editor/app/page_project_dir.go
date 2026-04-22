package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
)

// PageProjectDir is /project-dir
type PageProjectDir struct{ App *App }

func (p PageProjectDir) GET(r *http.Request) (
	body templ.Component,
	redirect string,
	enableBackgroundStreaming bool,
	disableRefreshAfterHidden bool,
	err error,
) {
	enableBackgroundStreaming = true
	disableRefreshAfterHidden = true

	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		redirect = href.PageBuildBundle()
		return
	}

	body = template.PageProjectDir(
		p.App.dir,
		p.App.initErr,
		p.App.repairErr,
		len(p.App.changed),
		p.App.numCorrupt,
		p.App.numMissing,
		p.App.sourceErrors,
		p.App.PickDirectory != nil,
	)
	return
}

func (PageProjectDir) OnPrefsChanged(
	event EventPrefsChanged, sse *datastar.ServerSentEventGenerator,
) error {
	return patchUIPrefs(sse, event)
}

// POSTPick is /project-dir/pick/{$}
//
// Only meaningful in hybrid (Wails) mode: opens the native OS directory
// picker and patches the selected path back into the folder signal so it
// populates the input field. No-op in web/server mode.
func (p PageProjectDir) POSTPick(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
) error {
	if p.App.PickDirectory == nil {
		return nil
	}
	picked, err := p.App.PickDirectory()
	if err != nil || picked == "" {
		return nil
	}
	if err := sse.MarshalAndPatchSignals(struct {
		Folder string `json:"folder"`
	}{Folder: picked}); err != nil {
		return err
	}
	if err := p.App.SetDir(picked); err != nil {
		return sse.Redirect(href.PageProjectDir())
	}
	return sse.Redirect(href.PageIndex())
}

// POSTOpen is /project-dir/open/{$}
//
// Opens the project from the folder path in the signal (populated either
// by the user typing in web mode, or by POSTPick's native dialog in
// hybrid mode).
func (p PageProjectDir) POSTOpen(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		Folder string `json:"folder"`
	},
) error {
	if signals.Folder == "" {
		return sse.Redirect(href.PageProjectDir())
	}
	if err := p.App.SetDir(signals.Folder); err != nil {
		return sse.Redirect(href.PageProjectDir())
	}
	return sse.Redirect(href.PageIndex())
}
