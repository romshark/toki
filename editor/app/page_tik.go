package app

import (
	"net/http"
	"slices"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
	"github.com/romshark/toki/editor/datapagesgen/httperr"
)

// PageTIK is /tik/{id}
type PageTIK struct{ App *App }

func (p PageTIK) GET(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
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

	if p.App.building {
		redirect = href.PageBuildBundle()
		return
	}

	p.App.clearBuildResultLocked()

	if p.App.mustRedirectToProjectDir() {
		redirect = href.PageProjectDir()
		return
	}

	iTIK := slices.IndexFunc(p.App.tiks, func(t *template.TIK) bool {
		return t.ID == path.ID
	})
	if iTIK == -1 {
		err = httperr.NotFound
		return
	}

	tk := p.App.orderTIK(p.App.tiks[iTIK])
	body = template.PageTIK(tk, p.App.OpenNewWindow != nil)
	return
}

func (p PageTIK) StreamOpen(
	r *http.Request,
	streamID uint64,
	signals struct {
		InstanceID string `json:"instance_id"`
	},
) error {
	tikID := r.PathValue("id")
	p.App.lock.Lock()
	defer p.App.lock.Unlock()
	p.App.registerTIKStreamLocked(streamID, signals.InstanceID, tikID)
	return nil
}

func (p PageTIK) StreamClose(r *http.Request, streamID uint64) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()
	p.App.unregisterTIKStreamLocked(streamID)
	return nil
}

func (p PageTIK) OnUpdated(
	event EventUpdated,
	sse *datastar.ServerSentEventGenerator,
	streamID uint64,
) error {
	p.App.lock.Lock()
	defer p.App.lock.Unlock()

	if p.App.building {
		return sse.Redirect(href.PageBuildBundle())
	}

	instID := p.App.streamInst[streamID]
	vs := p.App.tikViews[instID]
	if vs == nil || vs.tikID == "" {
		return nil
	}

	iTIK := slices.IndexFunc(p.App.tiks, func(t *template.TIK) bool {
		return t.ID == vs.tikID
	})
	if iTIK == -1 {
		return nil
	}

	var exclude string
	if event.SourceInstanceID != "" && event.SourceInstanceID == instID {
		exclude = event.ChangedEditor
	}

	tk := p.App.orderTIK(p.App.tiks[iTIK])
	// Patch signals before morphing the DOM. The morph contains elements
	// with data-attr:value bindings that read from these signals, so a
	// stale signal during the morph would cause Datastar to overwrite
	// the morphed value with the old one.
	if err := sse.MarshalAndPatchSignals(editorSignalsFor([]template.TIK{*tk}, exclude)); err != nil {
		return err
	}
	return sse.PatchElementTempl(template.TIKContent(tk, p.App.OpenNewWindow != nil))
}

func (PageTIK) OnReset(
	event EventReset,
	sse *datastar.ServerSentEventGenerator,
) error {
	if event.ResetEditor == "" {
		return nil
	}
	return sse.MarshalAndPatchSignals(struct {
		ResetDoneTIKID  string                       `json:"resetdonetikid"`
		ResetDoneLocale string                       `json:"resetdonelocale"`
		Editor          map[string]map[string]string `json:"editor"`
	}{
		ResetDoneTIKID:  event.TIKID,
		ResetDoneLocale: event.Locale,
		Editor: map[string]map[string]string{
			event.TIKID: {event.Locale: event.ResetValue},
		},
	})
}
