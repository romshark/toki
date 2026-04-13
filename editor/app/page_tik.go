package app

import (
	"fmt"
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

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
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
	body = template.PageTIK(tk)
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
		return sse.ExecuteScript(navigate(href.PageBuildBundle()))
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
	if err := sse.PatchElementTempl(template.TIKContent(tk)); err != nil {
		return err
	}
	return sse.ExecuteScript(syncEditorsScript([]template.TIK{*tk}, exclude))
}

func (PageTIK) OnReset(
	event EventReset,
	sse *datastar.ServerSentEventGenerator,
) error {
	if event.ResetEditor == "" {
		return nil
	}
	if err := sse.MarshalAndPatchSignals(struct {
		ResetDoneTIKID  string `json:"resetdonetikid"`
		ResetDoneLocale string `json:"resetdonelocale"`
	}{
		ResetDoneTIKID:  event.TIKID,
		ResetDoneLocale: event.Locale,
	}); err != nil {
		return err
	}
	return sse.ExecuteScript(fmt.Sprintf(
		"resetEditorValue(%q,%q)", event.ResetEditor, event.ResetValue,
	))
}
