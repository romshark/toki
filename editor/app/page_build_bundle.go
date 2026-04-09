package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
)

// PageBuildBundle is /build-bundle
type PageBuildBundle struct{ App *App }

func (p PageBuildBundle) GET(
	r *http.Request,
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

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		redirect = href.PageProjectDir()
		return
	}

	state := p.App.buildBundleStateLocked()

	// If not building and no result to show, redirect to dashboard.
	if !state.Building && state.Duration == 0 && state.Err == "" {
		if len(p.App.changed) == 0 || !p.App.canApplyChangesLocked() {
			redirect = href.PageIndex()
			return
		}
		// Show "Building..." — the actual build starts in StreamOpen
		// once the SSE stream is connected, ensuring the user sees
		// the loading state before the build begins.
		state.Building = true
	}

	body = template.PageBuildBundle(state)
	return
}

func (p PageBuildBundle) StreamOpen(
	r *http.Request,
	streamID uint64,
	signals struct {
		InstanceID string `json:"instance_id"`
	},
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()
	// Start the build now that the SSE stream is connected.
	// This guarantees the client sees the loading state before the build runs.
	if !p.App.building && p.App.buildDuration == 0 && p.App.buildErr == "" &&
		len(p.App.changed) > 0 && p.App.canApplyChangesLocked() {
		p.App.startBuildBundleLocked()
	}
	return nil
}

func (PageBuildBundle) StreamClose(r *http.Request, streamID uint64) error {
	return nil
}

func (p PageBuildBundle) OnUpdated(
	event EventUpdated,
	sse *datastar.ServerSentEventGenerator,
	streamID uint64,
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	state := p.App.buildBundleStateLocked()
	return sse.PatchElementTempl(template.PageBuildBundle(state))
}

func (a *App) buildBundleStateLocked() template.BuildBundleState {
	return template.BuildBundleState{
		Building:     a.building,
		Err:          a.buildErr,
		Duration:     a.buildDuration,
		TotalChanges: len(a.changed),
	}
}
