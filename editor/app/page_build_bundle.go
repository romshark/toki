package app

import (
	"context"
	"net/http"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app/datapagesgen/href"
	"github.com/romshark/toki/editor/app/template"
)

// PageBuildBundle is /build-bundle
type PageBuildBundle struct {
	App *App
	PrefsSync
}

func (p PageBuildBundle) GET(
	r *http.Request,
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

	if p.App.mustRedirectToProjectDir() {
		redirect.URL = href.PageProjectDir()
		return
	}

	state := p.App.buildBundleStateLocked()

	// If not building and no result to show, redirect to dashboard.
	if !state.Building && state.Duration == 0 && state.Err == "" {
		if len(p.App.changed) == 0 || !p.App.canApplyChangesLocked() {
			redirect.URL = href.PageIndex()
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
	_ datapages.StreamID,
	updated datapages.Dispatcher[EventUpdated],
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()
	// Start the build now that the SSE stream is connected.
	// This guarantees the client sees the loading state before the build runs.
	if !p.App.building && p.App.buildDuration == 0 && p.App.buildErr == "" &&
		len(p.App.changed) > 0 && p.App.canApplyChangesLocked() {
		p.App.startBuildBundleLocked(context.WithoutCancel(r.Context()), updated)
	}
	return nil
}

func (p PageBuildBundle) OnUpdated(event EventUpdated, sse datapages.SSE) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	return sse.PatchElement(template.PageBuildBundle(p.App.buildBundleStateLocked()))
}

func (a *App) buildBundleStateLocked() template.BuildBundleState {
	return template.BuildBundleState{
		Building:     a.building,
		Err:          a.buildErr,
		Duration:     a.buildDuration,
		TotalChanges: len(a.changed),
	}
}
