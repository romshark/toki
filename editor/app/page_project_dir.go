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

	body = template.PageProjectDir(p.App.dir, p.App.initErr, p.App.repairErr, len(p.App.changed), p.App.numCorrupt)
	return
}

// POSTOpen is /project-dir/open/{$}
//
// In Wails mode, PickDirectory opens a native OS directory picker dialog
// and the selected path overrides the folder signal.
// In web/server mode, PickDirectory is nil so the folder path comes
// from the text input signal.
func (p PageProjectDir) POSTOpen(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		Folder string `json:"folder"`
	},
) error {
	folder := signals.Folder
	if p.App.PickDirectory != nil {
		picked, err := p.App.PickDirectory()
		if err != nil || picked == "" {
			return sse.ExecuteScript(navigate(href.PageProjectDir()))
		}
		folder = picked
	}
	if folder == "" {
		return sse.ExecuteScript(navigate(href.PageProjectDir()))
	}
	if err := p.App.SetDir(folder); err != nil {
		return sse.ExecuteScript(navigate(href.PageProjectDir()))
	}
	return sse.ExecuteScript(navigate("/tiks/"))
}
