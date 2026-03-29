package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/icu"
	tikutil "github.com/romshark/toki/internal/tik"
	"github.com/starfederation/datastar-go/datastar"
	"golang.org/x/text/language"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
	"github.com/romshark/toki/editor/datapagesgen/httperr"
)

const MainBundleFileGo = "bundle_gen.go"

// EventUpdated is "editor.updated"
type EventUpdated struct{}

// EventReset is "editor.reset"
type EventReset struct {
	// ResetEditor is set after a reset to force-update the CodeMirror content.
	ResetEditor string `json:"reseteditor"`
	ResetValue  string `json:"resetvalue"`
	TIKID       string `json:"tikid"`
	Locale      string `json:"locale"`
}

// viewState holds per-instance server-side view parameters so that
// OnUpdated handlers can fat-morph each stream with the correct filters.
type viewState struct {
	filterType  string
	showLocales map[string]bool
	tikID       string // only used by PageTIK streams
	windowStart int
	refCount    int
}

type App struct {
	mu               sync.Mutex
	env              []string
	dir              string
	bundlePkgPath    string
	hasher           *xxhash.Digest
	icuTokenizer     *icumsg.Tokenizer
	icuTokBuffer     []icumsg.Token
	tikParser        *tik.Parser
	tikICUTranslator *tik.ICUTranslator
	scan             *codeparse.Scan
	tiks             []*template.TIK
	catalogs         []*template.Catalog
	localeTags       []language.Tag
	changed          []*template.ICUMessage
	initErr          string

	// views maps instance_id -> view state
	views map[string]*viewState
	// streamInst maps streamID -> instance_id
	streamInst map[uint64]string

	// PickDirectory opens a native directory picker dialog.
	// Set by the Wails runner; nil in server mode.
	PickDirectory func() (string, error)
}

func NewApp(dir, bundlePkgPath string, env []string) *App {
	return &App{
		dir:              dir,
		bundlePkgPath:    bundlePkgPath,
		env:              env,
		hasher:           xxhash.New(),
		icuTokenizer:     new(icumsg.Tokenizer),
		tikParser:        tik.NewParser(tik.DefaultConfig),
		tikICUTranslator: tik.NewICUTranslator(tik.DefaultConfig),
		views:            make(map[string]*viewState),
		streamInst:       make(map[uint64]string),
	}
}

func (a *App) Dir() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dir
}

func (a *App) InitErr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.initErr
}

func (a *App) TryInit() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tryInitLocked()
}

func (a *App) tryInitLocked() error {
	a.initErr = ""
	a.changed = nil
	a.tiks = nil
	a.catalogs = nil
	a.localeTags = nil
	a.scan = nil

	if a.dir == "" {
		a.initErr = "No directory selected"
		return errors.New(a.initErr)
	}

	mainBundleFile := filepath.Join(a.dir, a.bundlePkgPath, MainBundleFileGo)
	switch _, err := os.Stat(mainBundleFile); {
	case errors.Is(err, os.ErrNotExist):
		a.initErr = fmt.Sprintf(
			"Bundle file %q not found. Run `toki generate` first.",
			mainBundleFile,
		)
		return errors.New(a.initErr)
	case err != nil:
		a.initErr = fmt.Sprintf("Checking bundle file %q: %v", mainBundleFile, err)
		return errors.New(a.initErr)
	}

	prevDir, err := os.Getwd()
	if err != nil {
		a.initErr = fmt.Sprintf("Getting working directory: %v", err)
		return errors.New(a.initErr)
	}
	if err := os.Chdir(a.dir); err != nil {
		a.initErr = fmt.Sprintf("Changing to directory %q: %v", a.dir, err)
		return errors.New(a.initErr)
	}
	defer func() { _ = os.Chdir(prevDir) }()

	parser := codeparse.NewParser(a.hasher, a.tikParser, a.tikICUTranslator)
	scan, err := parser.Parse(a.env, "./...", a.bundlePkgPath, false)
	if err != nil {
		a.initErr = fmt.Sprintf("Analyzing source: %v", err)
		return errors.New(a.initErr)
	}
	if scan.SourceErrors.Len() > 0 {
		a.initErr = "Source code contains errors"
		return errors.New(a.initErr)
	}

	a.scan = scan
	a.localeTags = make([]language.Tag, 0, scan.Catalogs.Len())
	a.catalogs = make([]*template.Catalog, 0, scan.Catalogs.Len())
	a.tiks = make([]*template.TIK, 0, scan.TextIndexByID.Len())

	for cat := range scan.Catalogs.SeqRead() {
		c := &template.Catalog{
			Locale:  cat.ARB.Locale.String(),
			Default: cat.ARB.Locale == scan.DefaultLocale,
		}
		a.catalogs = append(a.catalogs, c)
		a.localeTags = append(a.localeTags, language.MustParse(c.Locale))
	}

	for _, i := range scan.TextIndexByID.SeqRead() {
		t := scan.Texts.At(i)
		tmplTIK := &template.TIK{
			ID:          t.IDHash,
			TIK:         t.TIK.Raw,
			Description: strings.Join(t.Comments, " "),
			ICU:         make([]*template.ICUMessage, 0, len(a.catalogs)),
		}
		for c := range scan.Catalogs.SeqRead() {
			m := c.ARB.Messages[t.IDHash]
			isReadOnly := false
			if c.ARB.Locale == scan.DefaultLocale {
				isReadOnly = tikutil.ProducesCompleteICU(c.ARB.Locale, t.TIK)
			}
			tmplMsg := &template.ICUMessage{
				ID: t.IDHash,
				Catalog: func() *template.Catalog {
					for i, c2 := range a.catalogs {
						if c.ARB.Locale == a.localeTags[i] {
							return c2
						}
					}
					return nil
				}(),
				Message:    m.ICUMessage,
				IsReadOnly: isReadOnly,
			}
			tmplTIK.ICU = append(tmplTIK.ICU, tmplMsg)
		}
		a.tiks = append(a.tiks, tmplTIK)
	}
	sort.Slice(a.tiks, func(i, j int) bool {
		return a.tiks[i].ID < a.tiks[j].ID
	})

	return nil
}

func (a *App) SetDir(dir string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dir = dir
	return a.tryInitLocked()
}

func (a *App) registerStreamLocked(streamID uint64, instanceID string, vs viewState) {
	a.streamInst[streamID] = instanceID
	if existing, ok := a.views[instanceID]; ok {
		existing.filterType = vs.filterType
		existing.showLocales = vs.showLocales
		if vs.tikID != "" {
			existing.tikID = vs.tikID
		}
		existing.refCount++
	} else {
		vs.refCount = 1
		a.views[instanceID] = &vs
	}
}

func (a *App) unregisterStreamLocked(streamID uint64) {
	instanceID, ok := a.streamInst[streamID]
	if !ok {
		return
	}
	delete(a.streamInst, streamID)
	if vs, ok := a.views[instanceID]; ok {
		vs.refCount--
		if vs.refCount <= 0 {
			delete(a.views, instanceID)
		}
	}
}

// initEditorsScript builds a JS expression that sets server-side editor values
// as a global and then calls initEditors(). This is needed because
// data-ignore-morph prevents the morph from updating textarea content,
// so initEditors() can't read the current server value from defaultValue.
func initEditorsScript(tiks []template.TIK) string {
	values := make(map[string]string, len(tiks)*2)
	for i := range tiks {
		for _, msg := range tiks[i].ICU {
			key := fmt.Sprintf("editor-%s-%s", tiks[i].ID, msg.Catalog.Locale)
			values[key] = msg.Message
		}
	}
	j, _ := json.Marshal(values)
	return fmt.Sprintf("window._serverValues=%s;initEditors()", j)
}

func (*App) Head(_ *http.Request) templ.Component { return template.Head() }

// POSTSet is /set/{$}
func (a *App) POSTSet(
	r *http.Request,
	dispatch func(EventUpdated) error,
	signals struct {
		TIKID  string `json:"settikid"`
		Locale string `json:"setlocale"`
		ICUMsg string `json:"icumsg"`
	},
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := signals.TIKID
	locale := signals.Locale
	newMessage := signals.ICUMsg

	if id == "" || locale == "" {
		return httperr.BadRequest
	}

	iCatalog := slices.IndexFunc(a.catalogs, func(c *template.Catalog) bool {
		return c.Locale == locale
	})
	if iCatalog == -1 {
		return httperr.BadRequest
	}
	iTIK := slices.IndexFunc(a.tiks, func(t *template.TIK) bool {
		return t.ID == id
	})
	if iTIK == -1 {
		return httperr.BadRequest
	}
	tk := a.tiks[iTIK]

	iICUMsg := slices.IndexFunc(tk.ICU, func(m *template.ICUMessage) bool {
		return m.Catalog == a.catalogs[iCatalog]
	})
	icuMsg := tk.ICU[iICUMsg]

	if icuMsg.Message != newMessage {
		loc := a.localeTags[iCatalog]

		var icuErr error
		a.icuTokBuffer = a.icuTokBuffer[:0]
		a.icuTokBuffer, icuErr = a.icuTokenizer.Tokenize(
			loc, a.icuTokBuffer, newMessage,
		)
		if icuErr != nil {
			icuMsg.Error = fmt.Sprintf("at index %d: %v", a.icuTokenizer.Pos(), icuErr)
		} else {
			icuMsg.Error = ""
			icuMsg.IncompleteReports = icu.AnalysisReport(
				loc, newMessage, a.icuTokBuffer, codeparse.ICUSelectOptions,
			)
		}

		if icuMsg.Changed {
			if newMessage == icuMsg.MessageOriginal {
				icuMsg.Message = newMessage
				icuMsg.Changed = false
				icuMsg.MessageOriginal = ""
				a.changed = slices.DeleteFunc(
					a.changed, func(m *template.ICUMessage) bool {
						return m == icuMsg
					})
			} else {
				icuMsg.Message = newMessage
			}
		} else {
			icuMsg.Changed = true
			icuMsg.MessageOriginal = icuMsg.Message
			icuMsg.Message = newMessage
			a.changed = append(a.changed, icuMsg)
		}
	}

	return dispatch(EventUpdated{})
}

// POSTReset is /reset/{$}
func (a *App) POSTReset(
	r *http.Request,
	dispatch func(EventUpdated, EventReset) error,
	signals struct {
		ResetTIKID  string `json:"resettikid"`
		ResetLocale string `json:"resetlocale"`
		InstanceID  string `json:"instance_id"`
	},
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := signals.ResetTIKID
	locale := signals.ResetLocale

	if id == "" || locale == "" {
		return httperr.BadRequest
	}

	iCatalog := slices.IndexFunc(a.catalogs, func(c *template.Catalog) bool {
		return c.Locale == locale
	})
	if iCatalog == -1 {
		return httperr.BadRequest
	}
	iTIK := slices.IndexFunc(a.tiks, func(t *template.TIK) bool {
		return t.ID == id
	})
	if iTIK == -1 {
		return httperr.BadRequest
	}
	tk := a.tiks[iTIK]

	iICUMsg := slices.IndexFunc(tk.ICU, func(m *template.ICUMessage) bool {
		return m.Catalog == a.catalogs[iCatalog]
	})
	icuMsg := tk.ICU[iICUMsg]

	resetEditorID := fmt.Sprintf("editor-%s-%s", id, locale)
	resetValue := icuMsg.MessageOriginal

	if icuMsg.Changed {
		icuMsg.Message = icuMsg.MessageOriginal
		icuMsg.Changed = false
		icuMsg.MessageOriginal = ""
		icuMsg.Error = ""
		icuMsg.IncompleteReports = nil
		a.changed = slices.DeleteFunc(a.changed, func(m *template.ICUMessage) bool {
			return m == icuMsg
		})
	}

	return dispatch(
		EventUpdated{},
		EventReset{
			ResetEditor: resetEditorID,
			ResetValue:  resetValue,
			TIKID:       id,
			Locale:      locale,
		},
	)
}

// POSTApplyChanges is /apply-changes/{$}
func (a *App) POSTApplyChanges(
	r *http.Request,
	dispatch func(EventUpdated) error,
	signals struct{},
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.canApplyChangesLocked() {
		return httperr.BadRequest
	}
	type ARBFile struct {
		*arb.File
		AbsolutePath string
		Changed      bool
	}

	arbFiles := make(map[string]ARBFile, a.scan.Catalogs.Len())
	for c := range a.scan.Catalogs.Seq() {
		arbFiles[c.ARB.Locale.String()] = ARBFile{
			File:         c.ARB,
			AbsolutePath: c.ARBFilePath,
		}
	}

	for _, c := range a.changed {
		arbFile := arbFiles[c.Catalog.Locale]
		arbFile.Changed = true
		m := arbFile.Messages
		arbMsg := m[c.ID]
		arbMsg.ICUMessage = c.Message
		m[c.ID] = arbMsg
		arbFiles[c.Catalog.Locale] = arbFile
	}

	for _, arbFile := range arbFiles {
		if !arbFile.Changed {
			continue
		}
		f, err := os.OpenFile(arbFile.AbsolutePath, os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("opening arb file: %w", err)
		}
		err = arb.Encode(f, arbFile.File, "\t")
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("encoding arb file: %w", err)
		}
	}

	if err := a.tryInitLocked(); err != nil {
		return err
	}

	return dispatch(EventUpdated{})
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	r *http.Request,
	query struct {
		Sidebar string `query:"s" reflectsignal:"sidebaropen"`
	},
) (
	body templ.Component,
	redirect string,
	err error,
) {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.dir == "" || p.App.initErr != "" {
		return nil, href.PageProjectDir(), nil
	}

	stats := p.App.buildDashboardStats()
	stats.SidebarOpen = query.Sidebar != "false"
	return template.PageDashboard(stats), "", nil
}

func (a *App) buildDashboardStats() template.DashboardStats {
	s := template.DashboardStats{
		Dir:             a.dir,
		NumTIKs:         len(a.tiks),
		NumLocales:      len(a.catalogs),
		TotalChanges:    len(a.changed),
		CanApplyChanges: a.canApplyChangesLocked(),
	}

	// Build per-locale stats.
	localeStats := make([]template.LocaleStats, len(a.catalogs))
	for i, c := range a.catalogs {
		localeStats[i] = template.LocaleStats{
			Locale:  c.Locale,
			Default: c.Default,
		}
	}

	for _, tk := range a.tiks {
		_, hasEmpty, hasIncomplete, hasInvalid := a.tikStatusFlags(tk, nil)

		for _, m := range tk.ICU {
			for li := range localeStats {
				if localeStats[li].Locale != m.Catalog.Locale {
					continue
				}
				if m.Message == "" {
					localeStats[li].Untranslated++
				} else {
					localeStats[li].Translated++
				}
				if m.Changed {
					localeStats[li].Changed++
				}
				break
			}
		}
		if hasEmpty {
			s.NumEmpty++
		}
		if hasIncomplete {
			s.NumIncomplete++
		} else {
			s.NumComplete++
		}
		if hasInvalid {
			s.NumInvalid++
		}
	}

	for i := range localeStats {
		total := localeStats[i].Translated + localeStats[i].Untranslated
		if total > 0 {
			localeStats[i].Completeness = float64(localeStats[i].Translated) / float64(total)
		}
	}
	s.Locales = localeStats

	if s.NumTIKs > 0 {
		s.Completeness = float64(s.NumComplete) / float64(s.NumTIKs)
	}
	return s
}

// PageTIKs is /tiks
type PageTIKs struct{ App *App }

func (p PageTIKs) GET(
	r *http.Request,
	query struct {
		Filter  string `query:"f" reflectsignal:"filtertype"`
		Locales string `query:"l" reflectsignal:"shownlocales"`
		Sidebar string `query:"s" reflectsignal:"sidebaropen"`
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

	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.dir == "" || p.App.initErr != "" {
		redirect = href.PageProjectDir()
		return
	}

	showLocales := parseLocalesParam(query.Locales)
	data := p.App.buildFilteredDataIndex(query.Filter, showLocales, 0)
	data.SidebarOpen = query.Sidebar != "false"
	body = template.PageTIKs(data)
	return
}

func (p PageTIKs) StreamOpen(
	r *http.Request,
	streamID uint64,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.registerStreamLocked(streamID, signals.InstanceID, viewState{
		filterType:  normalizeFilterType(signals.FilterType),
		showLocales: signals.ShowLocales,
	})
	return nil
}

func (p PageTIKs) StreamClose(r *http.Request, streamID uint64) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.unregisterStreamLocked(streamID)
	return nil
}

func (p PageTIKs) OnUpdated(
	event EventUpdated,
	sse *datastar.ServerSentEventGenerator,
	streamID uint64,
) error {
	_ = event
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.dir == "" || p.App.initErr != "" {
		return nil
	}

	instID := p.App.streamInst[streamID]
	vs := p.App.views[instID]
	if vs == nil {
		return nil
	}

	data := p.App.buildFilteredDataIndex(vs.filterType, vs.showLocales, vs.windowStart)
	if err := sse.PatchElementTempl(template.IndexContent(data)); err != nil {
		return err
	}
	return sse.ExecuteScript(initEditorsScript(data.TIKs))
}

func (PageTIKs) OnReset(
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

// POSTFilter is /tiks/filter/{$}
func (p PageTIKs) POSTFilter(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.dir == "" || p.App.initErr != "" {
		return httperr.BadRequest
	}

	ft := normalizeFilterType(signals.FilterType)
	windowStart := 0
	vs := p.App.views[signals.InstanceID]
	if vs != nil {
		// Reset window to 0 only when filter type changes.
		// Locale toggles preserve the scroll position.
		if vs.filterType != ft {
			vs.windowStart = 0
		}
		windowStart = vs.windowStart
		vs.filterType = ft
		vs.showLocales = signals.ShowLocales
	}

	data := p.App.buildFilteredDataIndex(ft, signals.ShowLocales, windowStart)
	if err := sse.PatchElementTempl(template.IndexContent(data)); err != nil {
		return err
	}
	return sse.ExecuteScript(initEditorsScript(data.TIKs))
}

// POSTScrollDown is /tiks/scroll-down/{$}
func (p PageTIKs) POSTScrollDown(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		WindowStart int             `json:"windowstart"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	return p.handleScroll(sse, signals.FilterType, signals.ShowLocales,
		signals.WindowStart, signals.InstanceID, false)
}

// POSTScrollUp is /tiks/scroll-up/{$}
func (p PageTIKs) POSTScrollUp(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		WindowStart int             `json:"windowstart"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	return p.handleScroll(sse, signals.FilterType, signals.ShowLocales,
		signals.WindowStart, signals.InstanceID, true)
}

func (p PageTIKs) handleScroll(
	sse *datastar.ServerSentEventGenerator,
	filterType string, showLocales map[string]bool,
	windowStart int, instanceID string,
	scrollUp bool,
) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.dir == "" || p.App.initErr != "" {
		return httperr.BadRequest
	}

	ft := normalizeFilterType(filterType)
	if vs := p.App.views[instanceID]; vs != nil {
		vs.windowStart = windowStart
	}

	data := p.App.buildFilteredDataIndex(ft, showLocales, windowStart)

	if scrollUp {
		// Before morph: find the first visible card and record its viewport
		// position so we can restore it after items are inserted above.
		if err := sse.ExecuteScript(
			`(function(){` +
				`var m=document.querySelector('#page-tiks main');` +
				`var cards=m.querySelectorAll('section.card[id]');` +
				`for(var i=0;i<cards.length;i++){` +
				`var r=cards[i].getBoundingClientRect();` +
				`if(r.bottom>0){window._tikAnchor={id:cards[i].id,top:r.top};return}}` +
				`})()`,
		); err != nil {
			return err
		}
	}

	if err := sse.PatchElementTempl(template.TiksContentList(data)); err != nil {
		return err
	}

	if scrollUp {
		// After morph: find the same card and scroll so it's at the
		// same viewport position as before.
		return sse.ExecuteScript(
			`if(window._tikAnchor&&window._tikAnchor.id){` +
				`var el=document.getElementById(window._tikAnchor.id);` +
				`if(el){` +
				`var m=document.querySelector('#page-tiks main');` +
				`m.scrollTop+=el.getBoundingClientRect().top-window._tikAnchor.top` +
				`}}`,
		)
	}
	return nil
}

func navigate(url string) string {
	return "window.location='" + url + "'"
}

// PageProjectDir is /project-dir
type PageProjectDir struct{ App *App }

func (p PageProjectDir) GET(r *http.Request) (
	body templ.Component,
	enableBackgroundStreaming bool,
	disableRefreshAfterHidden bool,
	err error,
) {
	enableBackgroundStreaming = true
	disableRefreshAfterHidden = true

	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	body = template.PageProjectDir(p.App.dir, p.App.initErr, len(p.App.changed))
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

// PageTIK is /tik/{id}
type PageTIK struct{ App *App }

func (p PageTIK) GET(
	r *http.Request,
	path struct {
		ID string `path:"id"`
	},
	query struct {
		Sidebar string `query:"s" reflectsignal:"sidebaropen"`
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

	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.dir == "" || p.App.initErr != "" {
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
	body = template.PageTIK(tk, query.Sidebar != "false")
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
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.registerStreamLocked(streamID, signals.InstanceID, viewState{
		tikID: tikID,
	})
	return nil
}

func (p PageTIK) StreamClose(r *http.Request, streamID uint64) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.unregisterStreamLocked(streamID)
	return nil
}

func (p PageTIK) OnUpdated(
	event EventUpdated,
	sse *datastar.ServerSentEventGenerator,
	streamID uint64,
) error {
	_ = event
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	instID := p.App.streamInst[streamID]
	vs := p.App.views[instID]
	if vs == nil || vs.tikID == "" {
		return nil
	}

	iTIK := slices.IndexFunc(p.App.tiks, func(t *template.TIK) bool {
		return t.ID == vs.tikID
	})
	if iTIK == -1 {
		return nil
	}

	tk := p.App.orderTIK(p.App.tiks[iTIK])
	if err := sse.PatchElementTempl(template.TIKContent(tk)); err != nil {
		return err
	}
	return sse.ExecuteScript(initEditorsScript([]template.TIK{*tk}))
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

// PageSettings is /settings
type PageSettings struct{ App *App }

func (PageSettings) GET(
	r *http.Request,
	query struct {
		Sidebar string `query:"s" reflectsignal:"sidebaropen"`
	},
) (body templ.Component, err error) {
	return template.PageSettings(query.Sidebar != "false"), nil
}

// Helper methods on App (must be called with mu held).

// orderTIK returns a copy of the TIK with ICU messages reordered (default locale first).
func (a *App) orderTIK(tk *template.TIK) *template.TIK {
	ordered := make([]*template.ICUMessage, 0, len(tk.ICU))
	for _, m := range tk.ICU {
		if m.Catalog.Default {
			ordered = append(ordered, m)
			break
		}
	}
	for _, m := range tk.ICU {
		if !m.Catalog.Default {
			ordered = append(ordered, m)
		}
	}
	return &template.TIK{
		ID:          tk.ID,
		TIK:         tk.TIK,
		Description: tk.Description,
		ICU:         ordered,
		IsChanged:   tk.IsChanged,
		IsInvalid:   tk.IsInvalid,
	}
}

// parseLocalesParam parses a comma-separated list of shown locales
// into the map format expected by buildFilteredDataIndex.
// Empty string means show all (nil map).
// "none" means show none (empty non-nil map).
func parseLocalesParam(s string) map[string]bool {
	if s == "" {
		return nil
	}
	if s == "-" {
		return map[string]bool{}
	}
	m := make(map[string]bool)
	for _, l := range strings.Split(s, ",") {
		l = strings.TrimSpace(l)
		if l != "" {
			m[l] = true
		}
	}
	return m
}

func normalizeFilterType(filterType string) string {
	if filterType == "" {
		return "all"
	}
	return filterType
}

func (a *App) canApplyChangesLocked() bool {
	for _, c := range a.changed {
		if c.Error != "" {
			return false
		}
	}
	return true
}

// buildTIKForDisplay builds a full TIK with ICU messages for display,
// filtering by shown locales and running ICU validation.
func (a *App) buildTIKForDisplay(
	tk *template.TIK, showLocales map[string]bool,
) template.TIK {
	isLocaleShown := func(locale string) bool {
		if showLocales == nil {
			return true
		}
		shown, ok := showLocales[locale]
		return ok && shown
	}

	tmplTIK := template.TIK{
		ID:          tk.ID,
		TIK:         tk.TIK,
		Description: tk.Description,
		ICU:         make([]*template.ICUMessage, 0, len(a.catalogs)),
	}
	for catIndex, c := range a.catalogs {
		if !isLocaleShown(c.Locale) {
			continue
		}
		src := func() *template.ICUMessage {
			for _, msg := range tk.ICU {
				if msg.Catalog == c {
					return msg
				}
			}
			return nil
		}()

		// Copy the message so display-time validation doesn't mutate
		// the shared state used by canApplyChangesLocked/POSTSet.
		m := *src
		var icuErr error
		a.icuTokBuffer = a.icuTokBuffer[:0]
		a.icuTokBuffer, icuErr = a.icuTokenizer.Tokenize(
			a.localeTags[catIndex], a.icuTokBuffer, m.Message,
		)
		if icuErr != nil {
			tmplTIK.IsInvalid = true
			m.Error = fmt.Sprintf("at index %d: %v", a.icuTokenizer.Pos(), icuErr)
		}
		m.IncompleteReports = icu.AnalysisReport(
			a.localeTags[catIndex], m.Message, a.icuTokBuffer,
			codeparse.ICUSelectOptions,
		)
		if len(m.IncompleteReports) != 0 {
			tmplTIK.IsIncomplete = true
			tmplTIK.IsInvalid = true
		}
		if m.Message == "" {
			tmplTIK.IsEmpty = true
			tmplTIK.IsIncomplete = true
		}
		if m.Changed {
			tmplTIK.IsChanged = true
		}
		tmplTIK.ICU = append(tmplTIK.ICU, &m)
	}
	tmplTIK.IsComplete = !tmplTIK.IsIncomplete
	return tmplTIK
}

// buildFilteredDataIndex builds a DataIndex with server-side filtering
// and virtual scroll windowing. Only TIKs within the window get full
// ICU validation; the rest are just counted for filter stats.
func (a *App) buildFilteredDataIndex(
	filterType string, showLocales map[string]bool, windowStart int,
) template.DataIndex {
	data := template.DataIndex{
		Dir:             a.dir,
		ShownLocales:    showLocales,
		FilterType:      filterType,
		CanApplyChanges: a.canApplyChangesLocked(),
		TotalChanges:    len(a.changed),
		WindowSize:      template.DefaultWindowSize,
	}

	// Move default catalog to first position in a copy,
	// so a.catalogs order stays in sync with a.localeTags.
	cats := make([]*template.Catalog, len(a.catalogs))
	copy(cats, a.catalogs)
	for i, c := range cats {
		if c.Default {
			cats[0], cats[i] = cats[i], cats[0]
			break
		}
	}
	data.Catalogs = cats

	// First pass: count stats and collect indices of filtered TIKs.
	filtered := make([]int, 0, len(a.tiks))
	for i, tk := range a.tiks {
		hasChanged, hasEmpty, hasIncomplete, hasInvalid := a.tikStatusFlags(tk, showLocales)
		isComplete := !hasIncomplete

		data.NumAll++
		if isComplete {
			data.NumComplete++
		}
		if hasIncomplete {
			data.NumIncomplete++
		}
		if hasEmpty {
			data.NumEmpty++
		}
		if hasInvalid {
			data.NumInvalid++
		}
		if hasChanged {
			data.NumChanged++
		}

		switch filterType {
		case "changed":
			if !hasChanged {
				continue
			}
		case "empty":
			if !hasEmpty {
				continue
			}
		case "complete":
			if !isComplete {
				continue
			}
		case "incomplete":
			if !hasIncomplete {
				continue
			}
		case "invalid":
			if !hasInvalid {
				continue
			}
		}
		filtered = append(filtered, i)
	}

	data.TotalFiltered = len(filtered)

	// Clamp window.
	if windowStart < 0 {
		windowStart = 0
	}
	if windowStart >= len(filtered) {
		windowStart = max(0, len(filtered)-data.WindowSize)
	}
	data.WindowStart = windowStart

	end := windowStart + data.WindowSize
	if end > len(filtered) {
		end = len(filtered)
	}

	// Second pass: build full TIKs only for the window.
	for _, idx := range filtered[windowStart:end] {
		tk := a.buildTIKForDisplay(a.tiks[idx], showLocales)
		data.TIKs = append(data.TIKs, tk)
	}

	return data
}

// tikStatusFlags computes status flags for a TIK without building the
// full display data. Used for fast counting in the first pass.
func (a *App) tikStatusFlags(
	tk *template.TIK, showLocales map[string]bool,
) (hasChanged, hasEmpty, hasIncomplete, hasInvalid bool) {
	for _, m := range tk.ICU {
		if showLocales != nil {
			if shown, ok := showLocales[m.Catalog.Locale]; !ok || !shown {
				continue
			}
		}
		if m.Message == "" {
			hasEmpty = true
			hasIncomplete = true
		}
		if m.Changed {
			hasChanged = true
		}
		if m.Error != "" {
			hasInvalid = true
		} else if m.Message != "" {
			// Look up the locale tag by catalog locale string.
			locTag := a.localeTagByLocale(m.Catalog.Locale)
			var icuErr error
			a.icuTokBuffer = a.icuTokBuffer[:0]
			a.icuTokBuffer, icuErr = a.icuTokenizer.Tokenize(
				locTag, a.icuTokBuffer, m.Message,
			)
			if icuErr != nil {
				hasInvalid = true
			} else if len(icu.AnalysisReport(
				locTag, m.Message, a.icuTokBuffer,
				codeparse.ICUSelectOptions,
			)) > 0 {
				hasIncomplete = true
				hasInvalid = true
			}
		}
	}
	return
}

func (a *App) localeTagByLocale(locale string) language.Tag {
	for i, c := range a.catalogs {
		if c.Locale == locale {
			return a.localeTags[i]
		}
	}
	return language.Und
}
