package app

import (
	"net/http"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app/datapagesgen/href"
	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/internal/tokibundle"
)

// PageTIK is /tik/{id}
type PageTIK struct {
	App *App
	PrefsSync
	ResetSync
}

// StateTIK is the per-tab view state of PageTIK.
type StateTIK struct {
	// TIKID is the TIK the tab shows, taken from the page URL.
	TIKID string
	// Reader renders the text of the tab.
	Reader tokibundle.Reader
}

func (p PageTIK) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (
	body datapages.Component,
	redirect datapages.Redirect,
	docAttrs datapages.DocumentAttrs,
	enableBackgroundStreaming datapages.EnableBackgroundStreaming,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	enableBackgroundStreaming = true
	disableRefreshAfterHidden = true
	tr := p.App.readerFor(r)
	docAttrs = documentAttrs(tr)

	if p.App.IsLoading() {
		body = template.PageLoading(tr)
		return
	}

	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		redirect.URL = href.PageBuildBundle()
		return
	}

	p.App.clearBuildResultLocked()

	if p.App.mustRedirectToProjectDir() {
		redirect.URL = href.PageProjectDir()
		return
	}

	tk := p.App.tikCopyLocked(path.Values.ID)
	if tk == nil {
		err = datapages.ErrNotFound
		return
	}
	body = template.PageTIK(tr, tk, p.App.OpenNewWindow != nil, readClosedSections(r))
	return
}

func (p PageTIK) StreamOpen(
	r *http.Request, state datapages.State[StateTIK],
) error {
	state.Values.TIKID = r.PathValue("id")
	state.Values.Reader = p.App.readerFor(r)
	return nil
}

func (p PageTIK) OnUpdated(
	event EventUpdated,
	sse datapages.SSE,
	state datapages.State[StateTIK],
	stateID string,
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return sse.Redirect(href.PageBuildBundle())
	}

	if p.App.mustRedirectToProjectDir() {
		return sse.Redirect(href.PageProjectDir())
	}

	tk := p.App.tikCopyLocked(state.Values.TIKID)
	if tk == nil {
		return nil
	}

	var exclude string
	if event.SourceStateID == stateID {
		exclude = event.ChangedEditor
	}

	// Signals first: the morph's data-attr:value bindings read them.
	if err := sse.PatchSignals(editorSignals{
		Editor: editorSignalsFor([]template.TIK{*tk}, exclude),
	}); err != nil {
		return err
	}
	// nil closed sections leave each section as the user set it.
	return sse.PatchElement(
		template.PageTIK(state.Values.Reader, tk, p.App.OpenNewWindow != nil, nil),
	)
}

// POSTSet is /tik/{id}/set/{$}
func (p PageTIK) POSTSet(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	query datapages.Query[struct {
		Locale string `query:"l"`
	}],
	signals datapages.Signals[struct {
		ICUMsg string `json:"icumsg"`
	}],
	_ datapages.State[StateTIK],
	stateID string,
	updated datapages.Dispatcher[EventUpdated],
) error {
	return p.App.setMessageFromTab(
		path.Values.ID, query.Values.Locale, signals.Values.ICUMsg,
		stateID, updated,
	)
}

// POSTSetSectionOpen is /tik/{id}/set-section-open/{$}
//
// A collapsible section of the sidebar sends whether the user opened or
// closed it. Its cookie keeps that for the next page load.
func (PageTIK) POSTSetSectionOpen(
	r *http.Request,
	_ datapages.Path[struct {
		ID string `path:"id"`
	}],
	sse datapages.SSE,
	signals datapages.Signals[struct {
		Section string `json:"section"`
		Open    bool   `json:"open"`
	}],
) error {
	return setSectionOpen(sse, signals.Values.Section, signals.Values.Open)
}

// tikCopyLocked returns a copy of TIK id, or nil when there is no such TIK.
// Caller holds the App lock.
func (a *App) tikCopyLocked(id string) *template.TIK {
	tk := a.tiksByID[id]
	if tk == nil {
		return nil
	}
	return copyTIK(tk)
}
