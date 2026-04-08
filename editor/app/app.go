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
	"sync/atomic"
	"time"

	"github.com/a-h/templ"
	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
	"github.com/romshark/toki/editor/datapagesgen/httperr"
	"github.com/romshark/toki/editor/indexdb"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/bundlerepair"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/icu"
	tikutil "github.com/romshark/toki/internal/tik"
	"github.com/starfederation/datastar-go/datastar"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

const MainBundleFileGo = "bundle_gen.go"

// EventUpdated is "editor.updated"
type EventUpdated struct {
	// SourceInstanceID is the tab instance that triggered the change.
	// Empty for non-editor events (e.g. file watcher, build).
	SourceInstanceID string `json:"source_instance_id"`
	// ChangedEditor is the editor ID (e.g. "editor-tikid-locale") that
	// was just changed. Excluded from syncEditorValues for the source
	// tab to avoid overwriting in-progress typing.
	ChangedEditor string `json:"changed_editor"`
}

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
	showDomains map[string]bool
	tikID       string // only used by PageTIK streams
	windowStart int
	searchQuery string
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
	domains          *codeparse.DomainTree
	tiks             []*template.TIK
	tiksByID         map[string]*template.TIK
	catalogs         []*template.Catalog
	localeTags       []language.Tag
	changed          []*template.ICUMessage
	numCorrupt       int // corrupt native locale messages (from scan or DB)
	repairErr        string
	initErr          string

	// loading is true while the index DB is being rebuilt in the background.
	loading atomic.Bool

	// indexDB is the SQLite index database. Nil if not configured.
	indexDB *indexdb.DB

	// Build bundle state (protected by mu).
	building      bool
	buildErr      string
	buildDuration time.Duration

	// views maps instance_id -> view state
	views map[string]*viewState
	// streamInst maps streamID -> instance_id
	streamInst map[uint64]string

	// PickDirectory opens a native directory picker dialog.
	// Set by the Wails runner; nil in server mode.
	PickDirectory func() (string, error)

	// Version is the Toki version string. Set by editor.Setup.
	Version string

	// SqinnPath is the path to the sqinn binary (with FTS5). Set by editor.Setup.
	SqinnPath string

	// CleanGenerated deletes stale generated Go files and creates a minimal
	// bundle so codeparse can succeed. Set by editor.Setup.
	CleanGenerated func(bundlePkgPath string, defaultLocale language.Tag) error

	// GenerateGoBundle generates the full Go bundle from a scan.
	// Set by editor.Setup.
	GenerateGoBundle func(bundlePkgPath string, scan *codeparse.Scan) error

	// NotifyUpdated publishes an EventUpdated through the message broker
	// so SSE streams get notified. Set by editor.Setup after server creation.
	NotifyUpdated func()
}

func NewApp(dir, bundlePkgPath string, env []string, db *indexdb.DB) *App {
	return &App{
		dir:              dir,
		bundlePkgPath:    bundlePkgPath,
		env:              env,
		indexDB:          db,
		hasher:           xxhash.New(),
		icuTokenizer:     new(icumsg.Tokenizer),
		tikParser:        tik.NewParser(tik.DefaultConfig),
		tikICUTranslator: tik.NewICUTranslator(tik.DefaultConfig),
		views:            make(map[string]*viewState),
		streamInst:       make(map[uint64]string),
	}
}

// IsLoading returns true while the index DB is being rebuilt.
func (a *App) IsLoading() bool {
	return a.loading.Load()
}

// SetLoading sets the loading state.
func (a *App) SetLoading(v bool) {
	a.loading.Store(v)
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
	a.numCorrupt = 0
	a.tiks = nil
	a.catalogs = nil
	a.localeTags = nil
	a.scan = nil
	a.domains = nil

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

	// Check if we can use the index DB (fast path).
	if a.indexDB != nil {
		bundleDir := filepath.Join(a.dir, a.bundlePkgPath)
		currentChecksum, err := indexdb.ComputeARBChecksum(bundleDir)
		if err == nil && currentChecksum != "" {
			storedChecksum, dbErr := a.indexDB.GetChecksum()
			schemaOK := a.indexDB.GetSchemaVersion() == indexdb.SchemaVersion
			storedVersion := a.indexDB.GetTokiVersion()
			versionOK := storedVersion == a.Version && storedVersion != "dev"
			if dbErr == nil && schemaOK && versionOK && currentChecksum == storedChecksum {
				// Fast path: checksums and schema version match, load from DB.
				if err := a.loadFromDBLocked(); err == nil {
					a.discoverDomainsLocked()
					return nil
				}
				// Fall through to full rebuild if DB load fails.
			}
		}
	}

	// Slow path: full source code parse + rebuild.
	if err := a.fullRebuildLocked(); err != nil {
		return err
	}
	a.discoverDomainsLocked()
	return nil
}

// discoverDomainsLocked discovers .tokidomain.yml files in the project directory.
func (a *App) discoverDomainsLocked() {
	if a.dir == "" {
		return
	}
	domains, err := codeparse.DiscoverDomains(a.dir)
	if err != nil {
		return // Non-fatal: domains page will just be empty.
	}
	a.domains = domains
}

// fullRebuildLocked runs full codeparse and rebuilds the index DB.
func (a *App) fullRebuildLocked() error {
	parser := codeparse.NewParser(a.hasher, a.tikParser, a.tikICUTranslator)
	scan, err := parser.Parse(a.env, a.dir, "./...", a.bundlePkgPath, false)
	if err != nil {
		a.initErr = fmt.Sprintf("Analyzing source: %v", err)
		return errors.New(a.initErr)
	}
	if scan.SourceErrors.Len() > 0 {
		a.initErr = "Source code contains errors"
		return errors.New(a.initErr)
	}

	a.scan = scan
	a.ensureNativeCatalogExists(scan)
	a.buildTemplateDataFromScanLocked()
	a.numCorrupt = a.nativeCatalogCorruptCount()

	// Populate the index DB from the scan results.
	if a.indexDB != nil {
		if err := a.populateDBFromScanLocked(); err != nil {
			a.initErr = fmt.Sprintf("Populating index DB: %v", err)
			return errors.New(a.initErr)
		}
	}

	return nil
}

// buildTemplateDataFromScanLocked builds template data structures from a.scan.
func (a *App) buildTemplateDataFromScanLocked() {
	scan := a.scan

	a.localeTags = make([]language.Tag, 0, scan.Catalogs.Len())
	a.catalogs = make([]*template.Catalog, 0, scan.Catalogs.Len())
	a.tiks = make([]*template.TIK, 0, scan.TextIndexByID.Len())

	for cat := range scan.Catalogs.SeqRead() {
		tag := language.MustParse(cat.ARB.Locale.String())
		c := &template.Catalog{
			Locale:  cat.ARB.Locale.String(),
			Name:    display.English.Languages().Name(tag),
			Default: cat.ARB.Locale == scan.DefaultLocale,
		}
		a.catalogs = append(a.catalogs, c)
		a.localeTags = append(a.localeTags, tag)
	}

	// Collect all source occurrences per TIK ID.
	dirPrefix := a.dir + string(filepath.Separator)
	occurrences := make(map[string][]template.SourceOccurrence)
	for t := range scan.Texts.SeqRead() {
		displayFile := t.Position.Filename
		if rel, ok := strings.CutPrefix(displayFile, dirPrefix); ok {
			displayFile = rel
		}
		occurrences[t.IDHash] = append(occurrences[t.IDHash], template.SourceOccurrence{
			File:        t.Position.Filename,
			DisplayFile: displayFile,
			Line:        t.Position.Line,
			Column:      t.Position.Column,
		})
	}

	for _, i := range scan.TextIndexByID.SeqRead() {
		t := scan.Texts.At(i)
		var domainFullName string
		if t.Domain != nil {
			var names []string
			for p := range t.Domain.Path() {
				names = append(names, p.Name)
			}
			slices.Reverse(names)
			domainFullName = strings.Join(names, ".")
		}
		tmplTIK := &template.TIK{
			ID:          t.IDHash,
			TIK:         t.TIK.Raw,
			Description: strings.Join(t.Comments, " "),
			Domain:      domainFullName,
			ICU:         make([]*template.ICUMessage, 0, len(a.catalogs)),
			Occurrences: occurrences[t.IDHash],
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
			// Validate ICU message on load.
			if tmplMsg.Message != "" {
				loc := language.MustParse(c.ARB.Locale.String())
				a.icuTokBuffer = a.icuTokBuffer[:0]
				var icuErr error
				a.icuTokBuffer, icuErr = a.icuTokenizer.Tokenize(
					loc, a.icuTokBuffer, tmplMsg.Message,
				)
				if icuErr != nil {
					tmplMsg.Error = fmt.Sprintf("at index %d: %v",
						a.icuTokenizer.Pos(), icuErr)
				} else {
					tmplMsg.IncompleteReports = icu.AnalysisReport(
						loc, tmplMsg.Message, a.icuTokBuffer,
						codeparse.ICUSelectOptions,
					)
				}
			}
			tmplTIK.ICU = append(tmplTIK.ICU, tmplMsg)
		}
		a.tiks = append(a.tiks, tmplTIK)
	}
	sort.Slice(a.tiks, func(i, j int) bool {
		return a.tiks[i].ID < a.tiks[j].ID
	})
	a.tiksByID = make(map[string]*template.TIK, len(a.tiks))
	for _, tk := range a.tiks {
		a.tiksByID[tk.ID] = tk
	}
}

// populateDBFromScanLocked saves the current scan data into the index DB.
func (a *App) populateDBFromScanLocked() error {
	db := a.indexDB

	if err := db.BeginTx(); err != nil {
		return err
	}
	defer func() { _ = db.Rollback() }()

	if err := db.Clear(); err != nil {
		return err
	}

	// Save catalogs.
	for i, c := range a.catalogs {
		var messagesCorrupt int
		if a.scan != nil {
			for sc := range a.scan.Catalogs.SeqRead() {
				if sc.ARB.Locale == a.localeTags[i] {
					messagesCorrupt = int(sc.MessagesCorrupt.Load())
					break
				}
			}
		}
		if err := db.InsertCatalog(indexdb.Catalog{
			Locale:          c.Locale,
			Name:            c.Name,
			IsDefault:       c.Default,
			MessagesCorrupt: messagesCorrupt,
		}); err != nil {
			return fmt.Errorf("inserting catalog %s: %w", c.Locale, err)
		}
	}

	// Save TIKs and messages.
	for _, tk := range a.tiks {
		if err := db.InsertTIK(indexdb.TIK{
			ID:          tk.ID,
			Raw:         tk.TIK,
			Description: tk.Description,
		}); err != nil {
			return fmt.Errorf("inserting TIK %s: %w", tk.ID, err)
		}
		for _, msg := range tk.ICU {
			if err := db.InsertMessage(indexdb.Message{
				TIKID:              tk.ID,
				Locale:             msg.Catalog.Locale,
				ICUMessage:         msg.Message,
				OriginalICUMessage: msg.Message,
				IsReadOnly:         msg.IsReadOnly,
			}); err != nil {
				return fmt.Errorf("inserting message %s/%s: %w",
					tk.ID, msg.Catalog.Locale, err)
			}
		}
	}

	// Populate FTS search index.
	for _, tk := range a.tiks {
		var sb strings.Builder
		sb.WriteString(tk.ID)
		sb.WriteByte(' ')
		sb.WriteString(tk.TIK)
		sb.WriteByte(' ')
		sb.WriteString(tk.Description)
		for _, msg := range tk.ICU {
			sb.WriteByte(' ')
			sb.WriteString(msg.Message)
		}
		_ = db.InsertSearchEntry(tk.ID, sb.String()) // best-effort
	}

	// Store checksum.
	bundleDir := filepath.Join(a.dir, a.bundlePkgPath)
	checksum, err := indexdb.ComputeARBChecksum(bundleDir)
	if err != nil {
		return fmt.Errorf("computing checksum: %w", err)
	}
	if err := db.SetChecksum(checksum); err != nil {
		return err
	}
	if err := db.SetSchemaVersion(indexdb.SchemaVersion); err != nil {
		return err
	}
	if err := db.SetTokiVersion(a.Version); err != nil {
		return err
	}

	return db.Commit()
}

// loadFromDBLocked loads catalogs, TIKs and messages from the index DB
// into in-memory template structures. This is the fast path that avoids
// the expensive codeparse step.
func (a *App) loadFromDBLocked() error {
	db := a.indexDB

	dbCatalogs, err := db.LoadCatalogs()
	if err != nil {
		return err
	}
	if len(dbCatalogs) == 0 {
		return errors.New("no catalogs in index DB")
	}

	dbTIKs, err := db.LoadTIKs()
	if err != nil {
		return err
	}

	dbMessages, err := db.LoadMessages()
	if err != nil {
		return err
	}

	// Build catalog map for quick lookup.
	a.catalogs = make([]*template.Catalog, 0, len(dbCatalogs))
	a.localeTags = make([]language.Tag, 0, len(dbCatalogs))
	catalogMap := make(map[string]*template.Catalog, len(dbCatalogs))
	for _, dc := range dbCatalogs {
		tag := language.MustParse(dc.Locale)
		c := &template.Catalog{
			Locale:  dc.Locale,
			Name:    dc.Name,
			Default: dc.IsDefault,
		}
		if dc.IsDefault {
			a.numCorrupt = dc.MessagesCorrupt
		}
		a.catalogs = append(a.catalogs, c)
		a.localeTags = append(a.localeTags, tag)
		catalogMap[dc.Locale] = c
	}

	// Group messages by TIK ID.
	msgsByTIK := make(map[string][]indexdb.Message, len(dbTIKs))
	for _, m := range dbMessages {
		msgsByTIK[m.TIKID] = append(msgsByTIK[m.TIKID], m)
	}

	// Build TIKs.
	a.tiks = make([]*template.TIK, 0, len(dbTIKs))
	for _, dt := range dbTIKs {
		tmplTIK := &template.TIK{
			ID:          dt.ID,
			TIK:         dt.Raw,
			Description: dt.Description,
			ICU:         make([]*template.ICUMessage, 0, len(a.catalogs)),
		}
		for _, dm := range msgsByTIK[dt.ID] {
			cat := catalogMap[dm.Locale]
			if cat == nil {
				continue
			}
			changed := dm.ICUMessage != dm.OriginalICUMessage
			tmplMsg := &template.ICUMessage{
				ID:         dt.ID,
				Catalog:    cat,
				Message:    dm.ICUMessage,
				IsReadOnly: dm.IsReadOnly,
				Changed:    changed,
			}
			if changed {
				tmplMsg.MessageOriginal = dm.OriginalICUMessage
				a.changed = append(a.changed, tmplMsg)
			}
			// Validate ICU message on load.
			if tmplMsg.Message != "" {
				loc := language.MustParse(dm.Locale)
				a.icuTokBuffer = a.icuTokBuffer[:0]
				var icuErr error
				a.icuTokBuffer, icuErr = a.icuTokenizer.Tokenize(
					loc, a.icuTokBuffer, tmplMsg.Message,
				)
				if icuErr != nil {
					tmplMsg.Error = fmt.Sprintf("at index %d: %v",
						a.icuTokenizer.Pos(), icuErr)
				} else {
					tmplMsg.IncompleteReports = icu.AnalysisReport(
						loc, tmplMsg.Message, a.icuTokBuffer,
						codeparse.ICUSelectOptions,
					)
				}
			}
			tmplTIK.ICU = append(tmplTIK.ICU, tmplMsg)
		}
		a.tiks = append(a.tiks, tmplTIK)
	}

	// TIKs are already sorted by ID from the DB query.
	a.tiksByID = make(map[string]*template.TIK, len(a.tiks))
	for _, tk := range a.tiks {
		a.tiksByID[tk.ID] = tk
	}
	return nil
}

func (a *App) SetDir(dir string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Close the old DB and open a new one for the new directory.
	if a.indexDB != nil {
		_ = a.indexDB.Close()
		a.indexDB = nil
	}
	a.dir = dir
	if dir != "" {
		dbPath := filepath.Join(dir, ".toki", "index.db")
		db, err := indexdb.Open(dbPath, a.SqinnPath)
		if err == nil {
			a.indexDB = db
		}
	}

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

// syncEditorsScript builds a JS call to syncEditorValues with current
// server-side values. This syncs editors that have data-ignore-morph
// (editable editors) after a morphdom patch.
// excludeEditor is omitted from the map so the source tab's in-progress
// typing is never overwritten by a stale echo.
func syncEditorsScript(tiks []template.TIK, excludeEditor string) string {
	values := make(map[string]string, len(tiks)*2)
	for i := range tiks {
		for _, msg := range tiks[i].ICU {
			key := fmt.Sprintf("editor-%s-%s", tiks[i].ID, msg.Catalog.Locale)
			if key == excludeEditor {
				continue
			}
			values[key] = msg.Message
		}
	}
	j, _ := json.Marshal(values)
	return fmt.Sprintf("syncEditorValues(%s)", j)
}

func (*App) Head(r *http.Request) templ.Component {
	p := ReadUIPrefs(r)
	return template.Head(template.UIPrefs{
		Theme:             p.Theme,
		UIFont:            p.UIFont,
		EditorFont:        p.EditorFont,
		UIFontSize:        p.UIFontSize,
		EditorFontSize:    p.EditorFontSize,
		IsDarkExpr:        p.IsDark(),
		UIFontCSS:         p.UIFontFamily(),
		EditorFontCSS:     p.EditorFontFamily(),
		UIFontSizeCSS:     p.UIFontSizeCSS(),
		EditorFontSizeCSS: p.EditorFontSizeCSS(),
	})
}

// POSTSet is /set/{$}
func (a *App) POSTSet(
	r *http.Request,
	dispatch func(EventUpdated) error,
	signals struct {
		TIKID      string `json:"settikid"`
		Locale     string `json:"setlocale"`
		ICUMsg     string `json:"icumsg"`
		InstanceID string `json:"instance_id"`
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

		// Persist to index DB.
		if a.indexDB != nil {
			_ = a.indexDB.UpdateMessage(id, locale, newMessage)
		}
	}

	return dispatch(EventUpdated{
		SourceInstanceID: signals.InstanceID,
		ChangedEditor:    fmt.Sprintf("editor-%s-%s", id, locale),
	})
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
		// Persist reset to index DB (restore original message).
		if a.indexDB != nil {
			_ = a.indexDB.UpdateMessage(id, locale, icuMsg.Message)
		}
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

	changed := make([]*template.ICUMessage, len(a.changed))
	copy(changed, a.changed)

	if err := a.doBuildBundleLocked(changed); err != nil {
		return err
	}

	return dispatch(EventUpdated{})
}

// clearBuildResultLocked clears stale build results so the build-bundle
// page doesn't show an old result when revisited later.
func (a *App) clearBuildResultLocked() {
	if !a.building {
		a.buildDuration = 0
		a.buildErr = ""
	}
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	r *http.Request,
) (
	body templ.Component,
	redirect string,
	err error,
) {
	if p.App.IsLoading() {
		return template.PageLoading(), "", nil
	}

	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.building {
		return nil, href.PageBuildBundle(), nil
	}

	p.App.clearBuildResultLocked()

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		return nil, href.PageProjectDir(), nil
	}

	stats := p.App.buildDashboardStats()
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
		var name string
		if tag, err := language.Parse(c.Locale); err == nil {
			name = display.English.Languages().Name(tag)
		}
		localeStats[i] = template.LocaleStats{
			Locale:  c.Locale,
			Name:    name,
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
				if m.Changed {
					localeStats[li].Changed++
				}
				if m.Message == "" {
					localeStats[li].Empty++
					localeStats[li].Incomplete++
				} else {
					// Check for ICU errors.
					locTag := a.localeTagByLocale(m.Catalog.Locale)
					var icuErr error
					a.icuTokBuffer = a.icuTokBuffer[:0]
					a.icuTokBuffer, icuErr = a.icuTokenizer.Tokenize(
						locTag, a.icuTokBuffer, m.Message,
					)
					hasErr := icuErr != nil ||
						m.Error != "" ||
						len(icu.AnalysisReport(
							locTag, m.Message, a.icuTokBuffer,
							codeparse.ICUSelectOptions,
						)) > 0
					if hasErr {
						localeStats[li].Invalid++
					} else {
						localeStats[li].Complete++
					}
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
		total := localeStats[i].Complete + localeStats[i].Empty + localeStats[i].Invalid
		if total > 0 {
			localeStats[i].Completeness = float64(localeStats[i].Complete) / float64(total)
		}
	}
	// Separate native (default) locale from the rest.
	for _, ls := range localeStats {
		if ls.Default {
			s.NativeLocale = ls
		} else {
			s.Locales = append(s.Locales, ls)
		}
	}

	if s.NumTIKs > 0 {
		s.Completeness = float64(s.NumComplete) / float64(s.NumTIKs)
	}

	// Count domains.
	if a.domains != nil {
		s.NumDomains = a.domains.Len()
	}

	return s
}

// nativeCatalogCorruptCount returns the MessagesCorrupt count from the scan's
// native catalog. Returns 0 if scan is nil or no native catalog exists.
func (a *App) nativeCatalogCorruptCount() int {
	if a.scan == nil {
		return 0
	}
	for c := range a.scan.Catalogs.SeqRead() {
		if c.ARB.Locale == a.scan.DefaultLocale {
			return int(c.MessagesCorrupt.Load())
		}
	}
	return 0
}

// ensureNativeCatalogExists creates an empty native catalog in the scan
// if the ARB file is missing. This allows detectCorruptMessages to flag
// every message as corrupt so the repair flow can regenerate the file.
func (a *App) ensureNativeCatalogExists(scan *codeparse.Scan) {
	for c := range scan.Catalogs.SeqRead() {
		if c.ARB.Locale == scan.DefaultLocale {
			return
		}
	}
	scan.Catalogs.Append(&codeparse.Catalog{
		ARB: &arb.File{
			Locale:   scan.DefaultLocale,
			Messages: make(map[string]arb.Message),
		},
		ARBFilePath: filepath.Join(
			a.dir, a.bundlePkgPath,
			fmt.Sprintf("catalog_%s.arb", scan.DefaultLocale),
		),
	})
}

// PageTIKs is /tiks
type PageTIKs struct{ App *App }

func (p PageTIKs) GET(
	r *http.Request,
	query struct {
		Filter  string `query:"f" reflectsignal:"filtertype"`
		Locales string `query:"l" reflectsignal:"shownlocales"`
		Domains string `query:"d" reflectsignal:"showndomains"`
		Search  string `query:"q" reflectsignal:"searchquery"`
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

	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.building {
		redirect = href.PageBuildBundle()
		return
	}

	p.App.clearBuildResultLocked()

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		redirect = href.PageProjectDir()
		return
	}

	showLocales := parseLocalesParam(query.Locales)
	showDomains := parseDomainsParam(query.Domains)
	data := p.App.buildFilteredDataIndex(query.Filter, showLocales, showDomains, 0, query.Search)
	body = template.PageTIKs(data)
	return
}

func (p PageTIKs) StreamOpen(
	r *http.Request,
	streamID uint64,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.registerStreamLocked(streamID, signals.InstanceID, viewState{
		filterType:  normalizeFilterType(signals.FilterType),
		showLocales: signals.ShowLocales,
		showDomains: normalizeDomainsSignal(signals.ShowDomains),
		searchQuery: signals.SearchQuery,
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
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.building {
		return sse.ExecuteScript(navigate(href.PageBuildBundle()))
	}

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		return nil
	}

	instID := p.App.streamInst[streamID]
	vs := p.App.views[instID]
	if vs == nil {
		return nil
	}

	// If this SSE connection belongs to the tab that triggered the
	// change, exclude the changed editor from the sync so that in-
	// progress typing is never overwritten by its own stale echo.
	var exclude string
	if event.SourceInstanceID != "" && event.SourceInstanceID == instID {
		exclude = event.ChangedEditor
	}

	data := p.App.buildFilteredDataIndex(vs.filterType, vs.showLocales, vs.showDomains, vs.windowStart, vs.searchQuery)
	if err := sse.PatchElementTempl(template.IndexContent(data)); err != nil {
		return err
	}
	return sse.ExecuteScript(syncEditorsScript(data.TIKs, exclude))
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
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		return httperr.BadRequest
	}

	ft := normalizeFilterType(signals.FilterType)
	windowStart := 0
	vs := p.App.views[signals.InstanceID]
	if vs != nil {
		// Reset window to 0 when filter type or search query changes.
		if vs.filterType != ft || vs.searchQuery != signals.SearchQuery {
			vs.windowStart = 0
		}
		windowStart = vs.windowStart
		vs.filterType = ft
		vs.showLocales = signals.ShowLocales
		vs.showDomains = normalizeDomainsSignal(signals.ShowDomains)
		vs.searchQuery = signals.SearchQuery
	}

	data := p.App.buildFilteredDataIndex(ft, signals.ShowLocales, normalizeDomainsSignal(signals.ShowDomains), windowStart, signals.SearchQuery)
	if err := sse.PatchElementTempl(template.IndexContent(data)); err != nil {
		return err
	}
	return sse.ExecuteScript(syncEditorsScript(data.TIKs, ""))
}

// POSTScrollDown is /tiks/scroll-down/{$}
func (p PageTIKs) POSTScrollDown(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		WindowStart int             `json:"windowstart"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	return p.handleScroll(sse, signals.FilterType, signals.ShowLocales, normalizeDomainsSignal(signals.ShowDomains),
		signals.SearchQuery, signals.WindowStart, signals.InstanceID, false)
}

// POSTScrollUp is /tiks/scroll-up/{$}
func (p PageTIKs) POSTScrollUp(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		FilterType  string          `json:"filtertype"`
		ShowLocales map[string]bool `json:"showlocales"`
		ShowDomains map[string]bool `json:"showdomains"`
		SearchQuery string          `json:"searchquery"`
		WindowStart int             `json:"windowstart"`
		InstanceID  string          `json:"instance_id"`
	},
) error {
	return p.handleScroll(sse, signals.FilterType, signals.ShowLocales, normalizeDomainsSignal(signals.ShowDomains),
		signals.SearchQuery, signals.WindowStart, signals.InstanceID, true)
}

func (p PageTIKs) handleScroll(
	sse *datastar.ServerSentEventGenerator,
	filterType string, showLocales, showDomains map[string]bool,
	searchQuery string, windowStart int, instanceID string,
	scrollUp bool,
) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.dir == "" || p.App.initErr != "" || p.App.numCorrupt > 0 {
		return httperr.BadRequest
	}

	ft := normalizeFilterType(filterType)
	if vs := p.App.views[instanceID]; vs != nil {
		vs.windowStart = windowStart
	}

	data := p.App.buildFilteredDataIndex(ft, showLocales, showDomains, windowStart, searchQuery)

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
	redirect string,
	enableBackgroundStreaming bool,
	disableRefreshAfterHidden bool,
	err error,
) {
	enableBackgroundStreaming = true
	disableRefreshAfterHidden = true

	p.App.mu.Lock()
	defer p.App.mu.Unlock()

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

	p.App.mu.Lock()
	defer p.App.mu.Unlock()

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
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.building {
		return sse.ExecuteScript(navigate(href.PageBuildBundle()))
	}

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

// PageDomains is /domains
type PageDomains struct{ App *App }

func (p PageDomains) GET(
	r *http.Request,
) (body templ.Component, redirect string, err error) {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.building {
		return nil, href.PageBuildBundle(), nil
	}
	if p.App.dir == "" || p.App.initErr != "" {
		return nil, href.PageProjectDir(), nil
	}

	data := p.App.buildDomainData()
	return template.PageDomains(data), "", nil
}

func (a *App) buildDomainData() template.DataDomains {
	data := template.DataDomains{
		Dir:             a.dir,
		TotalChanges:    len(a.changed),
		CanApplyChanges: a.canApplyChangesLocked(),
	}

	if a.domains == nil {
		return data
	}

	// Build per-domain stats from template TIKs.
	// Map TIK ID -> domain dir (only available with full scan).
	tikDomainDir := make(map[string]string)
	if a.scan != nil {
		for i := range a.scan.Texts.Len() {
			t := a.scan.Texts.At(i)
			if t.Domain != nil {
				tikDomainDir[t.IDHash] = t.Domain.Dir
			}
		}
	}

	type domainStats struct {
		numTIKs, complete, incomplete, empty, invalid, changed int
	}
	statsByDir := make(map[string]*domainStats)

	for _, tk := range a.tiks {
		dir, ok := tikDomainDir[tk.ID]
		if !ok {
			continue
		}
		ds := statsByDir[dir]
		if ds == nil {
			ds = &domainStats{}
			statsByDir[dir] = ds
		}
		ds.numTIKs++
		if tk.IsChanged {
			ds.changed++
		}
		if tk.IsComplete {
			ds.complete++
		}
		if tk.IsIncomplete {
			ds.incomplete++
		}
		if tk.IsEmpty {
			ds.empty++
		}
		if tk.IsInvalid {
			ds.invalid++
		}
	}

	// Build domain info tree from the DomainTree roots.
	var buildInfo func(d *codeparse.Domain) template.DomainInfo
	buildInfo = func(d *codeparse.Domain) template.DomainInfo {
		// Build full name from path iterator.
		var names []string
		for p := range d.Path() {
			names = append(names, p.Name)
		}
		slices.Reverse(names)
		fullName := strings.Join(names, ".")

		info := template.DomainInfo{
			Name:        d.Name,
			Description: d.Description,
			Dir:         d.Dir,
			FullName:    fullName,
		}
		if d.Parent != nil {
			info.ParentName = d.Parent.Name
			var parentNames []string
			for p := range d.Parent.Path() {
				parentNames = append(parentNames, p.Name)
			}
			slices.Reverse(parentNames)
			info.ParentFullName = strings.Join(parentNames, ".")
		}
		if ds := statsByDir[d.Dir]; ds != nil {
			info.NumTIKs = ds.numTIKs
			info.NumComplete = ds.complete
			info.NumIncomplete = ds.incomplete
			info.NumEmpty = ds.empty
			info.NumInvalid = ds.invalid
			info.NumChanged = ds.changed
			if ds.numTIKs > 0 {
				info.Completeness = float64(ds.complete) / float64(ds.numTIKs)
			}
		}
		for _, sub := range d.SubDomains {
			info.SubDomains = append(info.SubDomains, buildInfo(sub))
		}
		return info
	}

	// Find root domains (no parent).
	for d := range a.domains.All() {
		if d.Parent == nil {
			data.Domains = append(data.Domains, buildInfo(d))
		}
	}

	data.TotalDomains = a.domains.Len()
	return data
}

// PageSettings is /settings
type PageSettings struct{ App *App }

func (p PageSettings) GET(
	r *http.Request,
) (body templ.Component, redirect string, err error) {
	p.App.mu.Lock()
	building := p.App.building
	p.App.mu.Unlock()
	if building {
		return nil, href.PageBuildBundle(), nil
	}

	preview := "The quick brown fox jumps over the lazy dog"
	icuPreview := "{ plural, one {# item} other {# items} }"
	prefs := ReadUIPrefs(r)
	data := template.DataSettingsPreview{
		Prefs: template.UIPrefs{
			Theme:          prefs.Theme,
			UIFont:         prefs.UIFont,
			EditorFont:     prefs.EditorFont,
			UIFontSize:     prefs.UIFontSize,
			EditorFontSize: prefs.EditorFontSize,
		},
		UIFonts: []template.FontOption{
			{Value: "system", Family: fontFamilies["system"], Label: "System Default", Preview: preview},
			{Value: "georgia", Family: "Georgia, 'Times New Roman', serif", Label: "Georgia", Preview: preview},
			{Value: "helvetica", Family: "'Helvetica Neue', Helvetica, Arial, sans-serif", Label: "Helvetica", Preview: preview},
		},
		EditorFonts: []template.FontOption{
			{Value: "mono-system", Family: "ui-monospace, 'SF Mono', 'Cascadia Code', monospace", Label: "System Mono", Preview: icuPreview},
			{Value: "mono-firacode", Family: "'Fira Code', monospace", Label: "Fira Code", Preview: icuPreview},
			{Value: "mono-monaco", Family: "Monaco, Consolas, monospace", Label: "Monaco", Preview: icuPreview},
			{Value: "mono-courier", Family: "'Courier New', Courier, monospace", Label: "Courier New", Preview: icuPreview},
		},
		UIPreviewTIK:   "{name, select, other {Welcome, {name}!}}",
		UIPreviewICUEN: "{name, select, other {Welcome back, {name}!}}",
		UIPreviewICUDE: "{name, select, other {Willkommen, {name}!}}",
		UIPreviewEditorText: "{count, plural,\n" +
			"  one {You have # new message}\n" +
			"  other {You have # new messages}\n}",
	}
	body = template.PageSettings(p.App.Version, data)
	return
}

// POSTSetPref is /settings/set-pref/{$}
func (p PageSettings) POSTSetPref(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		PrefKey   string `json:"prefkey"`
		PrefValue string `json:"prefvalue"`
	},
) error {
	if !isValidUIPref(signals.PrefKey, signals.PrefValue) {
		return httperr.BadRequest
	}

	switch signals.PrefKey {
	case "toki-theme":
		if err := sse.MarshalAndPatchSignals(struct {
			PrefTheme string `json:"pref_theme"`
		}{
			PrefTheme: signals.PrefValue,
		}); err != nil {
			return err
		}
	case "toki-ui-font":
		if err := sse.MarshalAndPatchSignals(struct {
			PrefUIFont string `json:"pref_ui_font"`
		}{
			PrefUIFont: signals.PrefValue,
		}); err != nil {
			return err
		}
	case "toki-editor-font":
		if err := sse.MarshalAndPatchSignals(struct {
			PrefEditorFont string `json:"pref_editor_font"`
		}{
			PrefEditorFont: signals.PrefValue,
		}); err != nil {
			return err
		}
	case "toki-ui-font-size":
		if err := sse.MarshalAndPatchSignals(struct {
			PrefUIFontSize string `json:"pref_ui_font_size"`
		}{
			PrefUIFontSize: signals.PrefValue,
		}); err != nil {
			return err
		}
	case "toki-editor-font-size":
		if err := sse.MarshalAndPatchSignals(struct {
			PrefEditorFontSize string `json:"pref_editor_font_size"`
		}{
			PrefEditorFontSize: signals.PrefValue,
		}); err != nil {
			return err
		}
	default:
		return httperr.BadRequest
	}

	return sse.ExecuteScript(applyUIPrefScript(signals.PrefKey, signals.PrefValue))
}

// PageError404 is /error404/
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body templ.Component, err error) {
	return template.PageNotFound(r.URL.Path), nil
}

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

	p.App.mu.Lock()
	defer p.App.mu.Unlock()

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
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.registerStreamLocked(streamID, signals.InstanceID, viewState{})

	// Start the build now that the SSE stream is connected.
	// This guarantees the client sees the loading state before the build runs.
	if !p.App.building && p.App.buildDuration == 0 && p.App.buildErr == "" &&
		len(p.App.changed) > 0 && p.App.canApplyChangesLocked() {
		p.App.startBuildBundleLocked()
	}
	return nil
}

func (p PageBuildBundle) StreamClose(r *http.Request, streamID uint64) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()
	p.App.unregisterStreamLocked(streamID)
	return nil
}

func (p PageBuildBundle) OnUpdated(
	event EventUpdated,
	sse *datastar.ServerSentEventGenerator,
	streamID uint64,
) error {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

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

// startBuildBundleLocked kicks off the bundle build in a background goroutine.
// Must be called with mu held. The goroutine acquires mu itself for the heavy work.
func (a *App) startBuildBundleLocked() {
	a.building = true
	a.buildErr = ""
	a.buildDuration = 0

	// Snapshot what we need before releasing the lock.
	changed := make([]*template.ICUMessage, len(a.changed))
	copy(changed, a.changed)

	// Notify all SSE streams so clients on other pages redirect
	// to the build-bundle page.
	if a.NotifyUpdated != nil {
		a.NotifyUpdated()
	}

	go a.runBuildBundle(changed)
}

func (a *App) runBuildBundle(changed []*template.ICUMessage) {
	start := time.Now()

	a.mu.Lock()
	defer a.mu.Unlock()
	defer func() {
		a.building = false
		a.buildDuration = time.Since(start)
		// Notify SSE streams so the build-bundle page updates.
		if a.NotifyUpdated != nil {
			a.NotifyUpdated()
		}
	}()

	if err := a.doBuildBundleLocked(changed); err != nil {
		a.buildErr = err.Error()
	}
}

// doBuildBundleLocked performs the actual build. Caller holds mu.
func (a *App) doBuildBundleLocked(changed []*template.ICUMessage) error {
	absBundlePkg := filepath.Join(a.dir, a.bundlePkgPath)

	// Step 1: Clean stale generated Go files so codeparse can succeed.
	if a.CleanGenerated != nil {
		if err := a.CleanGenerated(absBundlePkg, a.scan.DefaultLocale); err != nil {
			return fmt.Errorf("cleaning generated files: %w", err)
		}
	}

	// Step 2: Run codeparse (reads current .arb files from disk).
	parser := codeparse.NewParser(a.hasher, a.tikParser, a.tikICUTranslator)
	scan, err := parser.Parse(a.env, a.dir, "./...", a.bundlePkgPath, false)
	if err != nil {
		return fmt.Errorf("analyzing source: %w", err)
	}

	// Step 3: Apply changed messages to the scan's ARB data.
	type arbFileEntry struct {
		*arb.File
		AbsolutePath string
		Changed      bool
	}
	arbFiles := make(map[string]arbFileEntry, scan.Catalogs.Len())
	for c := range scan.Catalogs.Seq() {
		arbFiles[c.ARB.Locale.String()] = arbFileEntry{
			File:         c.ARB,
			AbsolutePath: c.ARBFilePath,
		}
	}
	for _, c := range changed {
		af := arbFiles[c.Catalog.Locale]
		af.Changed = true
		msg := af.Messages[c.ID]
		msg.ICUMessage = c.Message
		af.Messages[c.ID] = msg
		arbFiles[c.Catalog.Locale] = af
	}

	// Step 4: Write updated .arb files.
	for _, af := range arbFiles {
		if !af.Changed {
			continue
		}
		f, err := os.OpenFile(af.AbsolutePath, os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("opening arb file: %w", err)
		}
		err = arb.Encode(f, af.File, "\t")
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("encoding arb file: %w", err)
		}
	}

	// Step 5: Generate Go code from the scan (which has the updated messages).
	if a.GenerateGoBundle != nil {
		if err := a.GenerateGoBundle(absBundlePkg, scan); err != nil {
			return fmt.Errorf("generating Go bundle: %w", err)
		}
	}

	// Step 6: Rebuild in-memory state from the scan.
	a.initErr = ""
	a.changed = nil
	a.scan = scan
	a.buildTemplateDataFromScanLocked()
	a.numCorrupt = a.nativeCatalogCorruptCount()

	if a.indexDB != nil {
		if err := a.populateDBFromScanLocked(); err != nil {
			return fmt.Errorf("populating index DB: %w", err)
		}
	}

	return nil
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
		Occurrences: tk.Occurrences,
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

// parseDomainsParam uses the same format as parseLocalesParam:
// "" = show all, "-" = show none, "a,b" = show listed.
func parseDomainsParam(s string) map[string]bool {
	return parseLocalesParam(s) // Same parsing logic.
}

// normalizeDomainsSignal converts Datastar signal keys (underscored)
// back to dotted full names used internally.
func normalizeDomainsSignal(m map[string]bool) map[string]bool {
	if m == nil {
		return nil
	}
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[strings.ReplaceAll(k, "_", ".")] = v
	}
	return out
}

func normalizeFilterType(filterType string) string {
	if filterType == "" {
		return "all"
	}
	return filterType
}

// RepairCorrupt repairs corrupt native locale messages by populating them
// from the TIK, writing the repaired ARB, and regenerating Go code.
// Existing unsaved user changes on other messages are preserved.
// On failure, the error is stored in repairErr for display on the project-dir page.
func (a *App) RepairCorrupt() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.repairErr = ""

	if a.numCorrupt == 0 {
		return
	}

	if err := a.repairCorruptLocked(); err != nil {
		a.repairErr = err.Error()
	}
}

func (a *App) repairCorruptLocked() error {
	// Snapshot existing user changes before repair rebuilds state.
	type savedChange struct {
		tikID, locale, message string
	}
	var userChanges []savedChange
	for _, c := range a.changed {
		userChanges = append(userChanges, savedChange{
			tikID:   c.ID,
			locale:  c.Catalog.Locale,
			message: c.Message,
		})
	}

	// Clean stale generated Go files so codeparse can succeed.
	absBundlePkg := filepath.Join(a.dir, a.bundlePkgPath)
	if a.CleanGenerated != nil {
		if err := a.CleanGenerated(absBundlePkg, a.scan.DefaultLocale); err != nil {
			return fmt.Errorf("cleaning generated files: %w", err)
		}
	}

	// Parse source to get a scan (needed for repair).
	parser := codeparse.NewParser(a.hasher, a.tikParser, a.tikICUTranslator)
	scan, err := parser.Parse(a.env, a.dir, "./...", a.bundlePkgPath, false)
	if err != nil {
		return fmt.Errorf("parsing source: %w", err)
	}
	a.ensureNativeCatalogExists(scan)

	// Fix corrupt messages in memory.
	repaired := bundlerepair.Repair(scan, a.tikICUTranslator, a.icuTokenizer)
	if len(repaired) == 0 {
		return nil
	}

	// Write the repaired native ARB to disk.
	for c := range scan.Catalogs.SeqRead() {
		if c.ARB.Locale != scan.DefaultLocale {
			continue
		}
		f, err := os.OpenFile(c.ARBFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("writing ARB: %w", err)
		}
		err = arb.Encode(f, c.ARB, "\t")
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("encoding ARB: %w", err)
		}
		break
	}

	// Regenerate Go code from the repaired scan.
	if a.GenerateGoBundle != nil {
		if err := a.GenerateGoBundle(absBundlePkg, scan); err != nil {
			return fmt.Errorf("generating Go bundle: %w", err)
		}
	}

	// Re-parse to get a clean scan (with correct checksums, tokens, etc.).
	scan, err = parser.Parse(a.env, a.dir, "./...", a.bundlePkgPath, false)
	if err != nil {
		return fmt.Errorf("re-parsing after repair: %w", err)
	}

	// Rebuild in-memory state from the repaired scan.
	a.scan = scan
	a.changed = nil
	a.buildTemplateDataFromScanLocked()
	a.numCorrupt = a.nativeCatalogCorruptCount()

	if a.indexDB != nil {
		if err := a.populateDBFromScanLocked(); err != nil {
			return fmt.Errorf("populating index DB: %w", err)
		}
	}

	// Restore user changes that were not part of the repair.
	for _, sc := range userChanges {
		for _, tk := range a.tiks {
			if tk.ID != sc.tikID {
				continue
			}
			for _, msg := range tk.ICU {
				if msg.Catalog.Locale != sc.locale {
					continue
				}
				msg.Changed = true
				msg.MessageOriginal = msg.Message
				msg.Message = sc.message
				a.changed = append(a.changed, msg)
				if a.indexDB != nil {
					_ = a.indexDB.UpdateMessage(sc.tikID, sc.locale, sc.message)
				}
				break
			}
			break
		}
	}

	return nil
}

func (a *App) canApplyChangesLocked() bool {
	for _, c := range a.changed {
		if c.Error != "" {
			return false
		}
	}
	return true
}

// buildTIKForDisplay builds a TIK for display, filtering by shown locales.
// Reads cached validation results from the ICUMessage (set at load/edit time).
func (a *App) buildTIKForDisplay(
	tk *template.TIK, showLocales map[string]bool,
) template.TIK {
	tmplTIK := template.TIK{
		ID:          tk.ID,
		TIK:         tk.TIK,
		Description: tk.Description,
		ICU:         make([]*template.ICUMessage, 0, len(a.catalogs)),
	}
	for _, c := range a.catalogs {
		if showLocales != nil {
			if shown, ok := showLocales[c.Locale]; !ok || !shown {
				continue
			}
		}
		for _, msg := range tk.ICU {
			if msg.Catalog != c {
				continue
			}
			if msg.Error != "" || len(msg.IncompleteReports) > 0 {
				tmplTIK.IsInvalid = true
			}
			if len(msg.IncompleteReports) > 0 {
				tmplTIK.IsIncomplete = true
			}
			if msg.Message == "" {
				tmplTIK.IsEmpty = true
				tmplTIK.IsIncomplete = true
			}
			if msg.Changed {
				tmplTIK.IsChanged = true
			}
			tmplTIK.ICU = append(tmplTIK.ICU, msg)
			break
		}
	}
	tmplTIK.IsComplete = !tmplTIK.IsIncomplete
	return tmplTIK
}

// buildFilteredDataIndex builds a DataIndex with server-side filtering
// and virtual scroll windowing. Only TIKs within the window get full
// ICU validation; the rest are just counted for filter stats.
//
// When searchQuery is non-empty, filter stats are skipped and results
// are fetched directly from the FTS5 index with LIMIT/OFFSET.
func (a *App) buildFilteredDataIndex(
	filterType string, showLocales, showDomains map[string]bool,
	windowStart int, searchQuery string,
) template.DataIndex {
	data := template.DataIndex{
		Dir:             a.dir,
		ShownLocales:    showLocales,
		ShownDomains:    showDomains,
		FilterType:      filterType,
		CanApplyChanges: a.canApplyChangesLocked(),
		TotalChanges:    len(a.changed),
		WindowSize:      template.DefaultWindowSize,
		SearchQuery:     searchQuery,
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

	// Populate domain filters.
	if a.domains != nil {
		for d := range a.domains.All() {
			var names []string
			for p := range d.Path() {
				names = append(names, p.Name)
			}
			slices.Reverse(names)
			fullName := strings.Join(names, ".")
			data.Domains = append(data.Domains, template.DomainFilter{
				FullName:  fullName,
				SignalKey: strings.ReplaceAll(fullName, ".", "_"),
				Name:      d.Name,
			})
		}
		sort.Slice(data.Domains, func(i, j int) bool {
			return data.Domains[i].FullName < data.Domains[j].FullName
		})
	}

	if searchQuery != "" && a.indexDB != nil {
		return a.buildSearchDataIndex(data, showLocales, windowStart, searchQuery)
	}
	return a.buildFilterDataIndex(data, showLocales, windowStart, filterType)
}

// buildSearchDataIndex handles the FTS search path — paginated DB query,
// no full iteration over a.tiks, no filter stats.
func (a *App) buildSearchDataIndex(
	data template.DataIndex, showLocales map[string]bool,
	windowStart int, searchQuery string,
) template.DataIndex {
	if windowStart < 0 {
		windowStart = 0
	}

	result, err := a.indexDB.SearchTIKs(searchQuery, windowStart, data.WindowSize)
	if err != nil {
		return data
	}

	data.TotalFiltered = result.Total
	if windowStart >= result.Total {
		windowStart = max(0, result.Total-data.WindowSize)
	}
	data.WindowStart = windowStart

	for _, id := range result.TIKIDs {
		if tk := a.tiksByID[id]; tk != nil {
			data.TIKs = append(data.TIKs, a.buildTIKForDisplay(tk, showLocales))
		}
	}

	return data
}

// buildFilterDataIndex handles the non-search path — full iteration
// over a.tiks with filter stats and virtual scroll windowing.
func (a *App) buildFilterDataIndex(
	data template.DataIndex, showLocales map[string]bool,
	windowStart int, filterType string,
) template.DataIndex {
	filtered := make([]int, 0, len(a.tiks))
	for i, tk := range a.tiks {
		// Domain filter: skip TIKs not in any shown domain.
		if data.ShownDomains != nil && !data.ShownDomains[tk.Domain] {
			continue
		}
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

	// Build full TIKs only for the window.
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
