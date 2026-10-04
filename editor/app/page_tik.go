package app

import (
	"net/http"
	"slices"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app/datapagesgen/href"
	"github.com/romshark/toki/editor/app/template"
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
}

func (p PageTIK) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
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

	if p.App.building {
		redirect.URL = href.PageBuildBundle()
		return
	}

	p.App.clearBuildResultLocked()

	if p.App.mustRedirectToProjectDir() {
		redirect.URL = href.PageProjectDir()
		return
	}

	tk := p.App.orderedTIKLocked(path.Values.ID)
	if tk == nil {
		err = datapages.ErrNotFound
		return
	}
	body = template.PageTIK(tk, p.App.OpenNewWindow != nil)
	return
}

func (PageTIK) StreamOpen(
	r *http.Request, state datapages.State[StateTIK],
) error {
	state.Values.TIKID = r.PathValue("id")
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

	tk := p.App.orderedTIKLocked(state.Values.TIKID)
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
	return sse.PatchElement(template.PageTIK(tk, p.App.OpenNewWindow != nil))
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

// orderedTIKLocked returns TIK id with its default-locale message first,
// or nil when there is no such TIK. Caller holds the App lock.
func (a *App) orderedTIKLocked(id string) *template.TIK {
	i := slices.IndexFunc(a.tiks, func(t *template.TIK) bool {
		return t.ID == id
	})
	if i == -1 {
		return nil
	}
	return a.orderTIK(a.tiks[i])
}
