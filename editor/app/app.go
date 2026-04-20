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
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/icu"
	tikutil "github.com/romshark/toki/internal/tik"
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

// pageTIKsState holds server-side view state for the TIKs list page.
type pageTIKsState struct {
	filterType  string
	showLocales map[string]bool
	showDomains map[string]bool
	pageIdx     int
	pageSize    int
	searchQuery string
	refCount    int
}

// pageTIKState holds server-side view state for a single TIK page.
type pageTIKState struct {
	tikID    string
	refCount int
}

// BundleEdit describes a pending in-memory edit to an ARB message.
// The editor collects these from user input and hands them to the
// bundle-build orchestrator, which persists them and regenerates Go code.
type BundleEdit struct {
	Locale  string // BCP 47 locale tag (matches ARB locale).
	TIKID   string // TIK message ID.
	Message string // Updated ICU message.
}

// domainStats aggregates per-domain TIK status counts.
type domainStats struct {
	numTIKs, complete, incomplete, untranslated, invalid, changed int
}

type App struct {
	lock             sync.Mutex
	env              []string
	dir              string
	bundlePkgPath    string
	hasher           *xxhash.Digest
	icuTokenizer     *icumsg.Tokenizer
	icuTokBuffer     []icumsg.Token
	tikParser        *tik.Parser
	tikICUTranslator *tik.ICUTranslator

	scan       *codeparse.Scan
	domains    *codeparse.DomainTree
	tiks       []*template.TIK
	tiksByID   map[string]*template.TIK
	catalogs   []*template.Catalog
	localeTags []language.Tag
	changed    []*template.ICUMessage
	// numCorrupt is the number of native-locale messages that
	// disagree with the TIK (needs `toki repair`)
	numCorrupt int
	// numMissing is the number of native-locale messages missing
	// from ARB (needs `toki generate`)
	numMissing int
	repairErr  string
	initErr    string

	// sourceErrors are populated when initErr is a source-error summary
	sourceErrors []template.SourceError

	// loading is true while the index DB is being rebuilt in the background.
	loading atomic.Bool

	// indexDB is the SQLite index database. Nil if not configured.
	indexDB *indexdb.DB

	// SqinnPath is the path to the sqinn binary (with FTS5). Set by editor.Setup.
	SqinnPath string

	// Build bundle state (protected by lock).
	building      bool
	buildErr      string
	buildDuration time.Duration

	// Per-page server-side state, keyed by instance_id.
	tiksViews  map[string]*pageTIKsState
	tikViews   map[string]*pageTIKState
	streamInst map[uint64]string // streamID -> instance_id (shared across page types)

	// PickDirectory opens a native directory picker dialog.
	// Set by the Wails runner; nil in server mode.
	PickDirectory func() (string, error)

	// Version is the Toki version string. Set by editor.Setup.
	Version string

	// CleanGenerated deletes stale generated Go files and creates a minimal
	// bundle so codeparse can succeed. Set by editor.Setup. Used by the
	// repair flow; the build flow goes through ApplyChangesAndBuild.
	CleanGenerated func(bundlePkgPath string, defaultLocale language.Tag) error

	// ApplyChangesAndBuild persists the supplied ARB edits and regenerates
	// the Go bundle. Set by editor.Setup. Returns the post-build scan so
	// the editor can refresh its in-memory state. See internal/app for
	// the actual implementation.
	ApplyChangesAndBuild func(
		env []string, modDir, bundlePkgPath string, defaultLocale language.Tag,
		edits []BundleEdit,
	) (*codeparse.Scan, error)

	// GenerateGoBundle regenerates the Go bundle from a scan produced by
	// a prior parse. Set by editor.Setup. Used by the repair flow only.
	GenerateGoBundle func(bundlePkgPath string, scan *codeparse.Scan) error

	// RepairBundle fixes corrupt native-locale entries by regenerating them
	// from TIK source. Set by editor.Setup. Returns the number of messages
	// repaired (0 if nothing was corrupt).
	RepairBundle func(env []string, modDir, bundlePkgPath string) (int, error)

	// RegenerateBundle runs `toki generate`'s logic programmatically to
	// bring the bundle back in sync with source code (adds missing native
	// ARB entries and regenerates Go). Set by editor.Setup.
	RegenerateBundle func(env []string, modDir, bundlePkgPath string) error
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
		tiksViews:        make(map[string]*pageTIKsState),
		tikViews:         make(map[string]*pageTIKState),
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
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.dir
}

func (a *App) InitErr() string {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.initErr
}

// TryInit loads the bundle for the configured project directory into the
// App's in-memory state. Takes the App lock, then delegates to
// [App.tryInitLocked]. On success the editor is ready to serve pages; on
// failure the reason is stored in initErr (plus optionally sourceErrors /
// numCorrupt / numMissing) so the project-dir page can surface it.
func (a *App) TryInit() error {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.tryInitLocked()
}

// mustRedirectToProjectDir reports whether the editor should redirect the user
// to the project-dir page instead of letting them edit translations.
// True when the bundle is missing, out-of-sync with source, or corrupt.
func (a *App) mustRedirectToProjectDir() bool {
	return a.dir == "" || a.initErr != "" || a.numCorrupt > 0 || a.numMissing > 0
}

// defaultLocaleLocked returns the bundle's native locale. Prefers the
// authoritative value from a.scan when the slow/full-rebuild path
// populated it; falls back to the Default catalog on the fast-path (DB)
// init where no scan exists. Returns language.Und if the bundle isn't
// loaded yet. Caller must hold a.lock.
func (a *App) defaultLocaleLocked() language.Tag {
	if a.scan != nil {
		return a.scan.DefaultLocale
	}
	for i, c := range a.catalogs {
		if c.Default {
			return a.localeTags[i]
		}
	}
	return language.Tag{}
}

// tryInitLocked resets all derived state and then tries to load the bundle:
// the fast path restores catalogs/TIKs/messages from the index DB when the
// ARB checksum + schema + toki version all match; otherwise it falls back
// to a full codeparse-driven rebuild. Domain discovery runs in both paths.
// Must be called with a.lock held.
func (a *App) tryInitLocked() error {
	a.initErr = ""
	a.sourceErrors = nil
	a.changed = nil
	a.numCorrupt = 0
	a.numMissing = 0
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
		_ = scan.SourceErrors.Access(func(s []codeparse.SourceError) error {
			a.sourceErrors = make([]template.SourceError, len(s))
			for i, se := range s {
				a.sourceErrors[i] = template.SourceError{
					File: se.Filename,
					Line: se.Line,
					Col:  se.Column,
					Err:  se.Err.Error(),
				}
			}
			return nil
		})
		return errors.New(a.initErr)
	}

	a.scan = scan
	a.ensureNativeCatalogExists(scan)
	a.buildTemplateDataFromScanLocked()
	a.numCorrupt, a.numMissing = a.nativeCatalogStatusCounts()

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
			Domain:      tk.Domain,
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
			Domain:      dt.Domain,
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
	a.lock.Lock()
	defer a.lock.Unlock()

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

func (a *App) registerTIKsStreamLocked(
	streamID uint64, instanceID string, vs pageTIKsState,
) {
	a.streamInst[streamID] = instanceID
	if existing, ok := a.tiksViews[instanceID]; ok {
		existing.filterType = vs.filterType
		existing.showLocales = vs.showLocales
		existing.showDomains = vs.showDomains
		existing.searchQuery = vs.searchQuery
		existing.pageIdx = vs.pageIdx
		existing.pageSize = vs.pageSize
		existing.refCount++
	} else {
		vs.refCount = 1
		a.tiksViews[instanceID] = &vs
	}
}

func (a *App) unregisterTIKsStreamLocked(streamID uint64) {
	instanceID, ok := a.streamInst[streamID]
	if !ok {
		return
	}
	delete(a.streamInst, streamID)
	if vs, ok := a.tiksViews[instanceID]; ok {
		vs.refCount--
		if vs.refCount <= 0 {
			delete(a.tiksViews, instanceID)
		}
	}
}

func (a *App) registerTIKStreamLocked(streamID uint64, instanceID string, tikID string) {
	a.streamInst[streamID] = instanceID
	if existing, ok := a.tikViews[instanceID]; ok {
		existing.tikID = tikID
		existing.refCount++
	} else {
		a.tikViews[instanceID] = &pageTIKState{tikID: tikID, refCount: 1}
	}
}

func (a *App) unregisterTIKStreamLocked(streamID uint64) {
	instanceID, ok := a.streamInst[streamID]
	if !ok {
		return
	}
	delete(a.streamInst, streamID)
	if vs, ok := a.tikViews[instanceID]; ok {
		vs.refCount--
		if vs.refCount <= 0 {
			delete(a.tikViews, instanceID)
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
	a.lock.Lock()
	defer a.lock.Unlock()

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
	a.lock.Lock()
	defer a.lock.Unlock()

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
	a.lock.Lock()
	defer a.lock.Unlock()

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

// POSTRepair is /repair/{$}
func (a *App) POSTRepair(
	r *http.Request,
	dispatch func(EventUpdated) error,
) (
	redirect string,
	err error,
) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.repairErr = ""

	if a.numCorrupt == 0 {
		return href.PageIndex(), nil
	}

	if err := a.repairCorruptLocked(); err != nil {
		a.repairErr = err.Error()
		return href.PageIndex(), nil
	}

	if err := dispatch(EventUpdated{}); err != nil {
		return "", err
	}
	return href.PageIndex(), nil
}

// POSTRegenerate is /regenerate/{$}
func (a *App) POSTRegenerate(
	r *http.Request,
	dispatch func(EventUpdated) error,
) (
	redirect string,
	err error,
) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.repairErr = ""

	if a.numMissing == 0 {
		return href.PageIndex(), nil
	}

	if err := a.regenerateLocked(); err != nil {
		a.repairErr = err.Error()
		return href.PageIndex(), nil
	}

	if err := dispatch(EventUpdated{}); err != nil {
		return "", err
	}
	return href.PageIndex(), nil
}

// clearBuildResultLocked clears stale build results so the build-bundle
// page doesn't show an old result when revisited later.
func (a *App) clearBuildResultLocked() {
	if !a.building {
		a.buildDuration = 0
		a.buildErr = ""
	}
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
		_, hasUntranslated, hasIncomplete, hasInvalid := a.tikStatusFlags(tk, nil)

		for _, m := range tk.ICU {
			for li := range localeStats {
				if localeStats[li].Locale != m.Catalog.Locale {
					continue
				}
				if m.Changed {
					localeStats[li].Changed++
				}
				if m.Message == "" {
					localeStats[li].Untranslated++
				} else {
					locTag := a.localeTagByLocale(m.Catalog.Locale)
					var icuErr error
					a.icuTokBuffer = a.icuTokBuffer[:0]
					a.icuTokBuffer, icuErr = a.icuTokenizer.Tokenize(
						locTag, a.icuTokBuffer, m.Message,
					)
					switch {
					case icuErr != nil || m.Error != "":
						localeStats[li].Invalid++
					case len(icu.AnalysisReport(
						locTag, m.Message, a.icuTokBuffer,
						codeparse.ICUSelectOptions,
					)) > 0:
						localeStats[li].Incomplete++
					default:
						localeStats[li].Complete++
					}
				}
				break
			}
		}
		if hasUntranslated {
			s.NumUntranslated++
		}
		if hasIncomplete {
			s.NumIncomplete++
		}
		if hasInvalid {
			s.NumInvalid++
		}
		if !hasUntranslated && !hasIncomplete && !hasInvalid {
			s.NumComplete++
		}
	}

	for i := range localeStats {
		total := localeStats[i].Complete +
			localeStats[i].Incomplete +
			localeStats[i].Untranslated +
			localeStats[i].Invalid
		if total > 0 {
			localeStats[i].Completeness = float64(localeStats[i].Complete) / float64(total)
		}
	}
	// Record the native (default) locale separately for the Overview card,
	// but include every locale (native included) in the full Locales grid.
	for _, ls := range localeStats {
		if ls.Default {
			s.NativeLocale = ls
		}
		s.Locales = append(s.Locales, ls)
	}

	if s.NumTIKs > 0 {
		s.Completeness = float64(s.NumComplete) / float64(s.NumTIKs)
	}

	// Build domain info.
	if a.domains != nil {
		s.NumDomains = a.domains.Len()
		domainData := a.buildDomainData()
		s.Domains = domainData.Domains
	}

	return s
}

// nativeCatalogStatusCounts returns (corrupt, missing) counts from the
// scan's native catalog. Zero for both when scan is nil or no native
// catalog exists. "Corrupt" means ARB entries disagree with the TIK;
// "missing" means TIKs exist in source but have no ARB entry.
func (a *App) nativeCatalogStatusCounts() (corrupt, missing int) {
	if a.scan == nil {
		return 0, 0
	}
	for c := range a.scan.Catalogs.SeqRead() {
		if c.ARB.Locale == a.scan.DefaultLocale {
			return int(c.MessagesCorrupt.Load()), int(c.MessagesMissing.Load())
		}
	}
	return 0, 0
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

// PageError404 is /error404/
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body templ.Component, err error) {
	return template.PageNotFound(r.URL.Path), nil
}

// startBuildBundleLocked kicks off the bundle build in a background goroutine.
// Must be called with lock held. The goroutine acquires lock itself for the heavy work.
// dispatch is captured from StreamOpen and used to notify SSE streams when the
// build starts and completes; the build goroutine outlives the triggering
// request, so the dispatch's request context may be canceled before the final
// event is emitted — the in-memory broker tolerates this.
func (a *App) startBuildBundleLocked(dispatch func(EventUpdated) error) {
	a.building = true
	a.buildErr = ""
	a.buildDuration = 0

	// Snapshot what we need before releasing the lock.
	changed := make([]*template.ICUMessage, len(a.changed))
	copy(changed, a.changed)

	// Notify all SSE streams so clients on other pages redirect
	// to the build-bundle page.
	_ = dispatch(EventUpdated{})

	go a.runBuildBundle(changed, dispatch)
}

func (a *App) runBuildBundle(
	changed []*template.ICUMessage, dispatch func(EventUpdated) error,
) {
	start := time.Now()

	a.lock.Lock()
	defer a.lock.Unlock()
	defer func() {
		a.building = false
		a.buildDuration = time.Since(start)
		// Notify SSE streams so the build-bundle page updates.
		_ = dispatch(EventUpdated{})
	}()

	if err := a.doBuildBundleLocked(changed); err != nil {
		a.buildErr = err.Error()
	}
}

// doBuildBundleLocked persists pending edits and rebuilds the bundle
// via the ApplyChangesAndBuild callback, then refreshes the editor's
// in-memory state from the returned scan. Caller holds lock.
func (a *App) doBuildBundleLocked(changed []*template.ICUMessage) error {
	edits := make([]BundleEdit, len(changed))
	for i, c := range changed {
		edits[i] = BundleEdit{
			Locale:  c.Catalog.Locale,
			TIKID:   c.ID,
			Message: c.Message,
		}
	}
	scan, err := a.ApplyChangesAndBuild(
		a.env, a.dir, a.bundlePkgPath, a.defaultLocaleLocked(), edits,
	)
	if err != nil {
		return err
	}

	a.initErr = ""
	a.changed = nil
	a.scan = scan
	a.buildTemplateDataFromScanLocked()
	a.numCorrupt, a.numMissing = a.nativeCatalogStatusCounts()

	if a.indexDB != nil {
		if err := a.populateDBFromScanLocked(); err != nil {
			return fmt.Errorf("populating index DB: %w", err)
		}
	}
	return nil
}

// Helper methods on App (must be called with lock held).

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
		Domain:      tk.Domain,
		ICU:         ordered,
		Occurrences: tk.Occurrences,
		IsChanged:   tk.IsChanged,
		IsInvalid:   tk.IsInvalid,
	}
}

func serializeShownSignal(m map[string]bool) string {
	if m == nil {
		return ""
	}
	var keys []string
	for k, v := range m {
		if v {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return "-"
	}
	return strings.Join(keys, ",")
}

// parseLocalesParam parses a comma-separated list of shown locales
// into the map format expected by buildFilteredDataIndex.
// Empty string means show all (nil map).
// "none" means show none (empty non-nil map).
// serializeShownSignal converts a shown map back to the string format
// used by the shownlocales/showndomains URL signals.
// nil = "" (show all), empty = "-" (show none), otherwise comma-separated keys.
func parseLocalesParam(s string) map[string]bool {
	if s == "" {
		return nil
	}
	if s == "-" {
		return map[string]bool{}
	}
	m := make(map[string]bool)
	for l := range strings.SplitSeq(s, ",") {
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

func (a *App) regenerateLocked() error {
	if a.RegenerateBundle == nil {
		return errors.New("regenerate is not available: no RegenerateBundle callback configured")
	}
	if err := a.RegenerateBundle(a.env, a.dir, a.bundlePkgPath); err != nil {
		return err
	}

	// Re-parse to rebuild in-memory state from the regenerated bundle.
	parser := codeparse.NewParser(a.hasher, a.tikParser, a.tikICUTranslator)
	scan, err := parser.Parse(a.env, a.dir, "./...", a.bundlePkgPath, false)
	if err != nil {
		return fmt.Errorf("re-parsing after regenerate: %w", err)
	}
	a.scan = scan
	a.changed = nil
	a.buildTemplateDataFromScanLocked()
	a.numCorrupt, a.numMissing = a.nativeCatalogStatusCounts()

	if a.indexDB != nil {
		if err := a.populateDBFromScanLocked(); err != nil {
			return fmt.Errorf("populating index DB: %w", err)
		}
	}
	return nil
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

	if a.RepairBundle == nil {
		return errors.New("repair is not available: no RepairBundle callback configured")
	}
	if _, err := a.RepairBundle(a.env, a.dir, a.bundlePkgPath); err != nil {
		return err
	}

	// Re-parse to rebuild in-memory state from the on-disk bundle.
	parser := codeparse.NewParser(a.hasher, a.tikParser, a.tikICUTranslator)
	scan, err := parser.Parse(a.env, a.dir, "./...", a.bundlePkgPath, false)
	if err != nil {
		return fmt.Errorf("re-parsing after repair: %w", err)
	}
	a.scan = scan
	a.changed = nil
	a.buildTemplateDataFromScanLocked()
	a.numCorrupt, a.numMissing = a.nativeCatalogStatusCounts()

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
		Domain:      tk.Domain,
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
			if msg.Error != "" {
				tmplTIK.IsInvalid = true
			}
			if len(msg.IncompleteReports) > 0 {
				tmplTIK.IsIncomplete = true
			}
			if msg.Message == "" {
				tmplTIK.IsUntranslated = true
			}
			if msg.Changed {
				tmplTIK.IsChanged = true
			}
			tmplTIK.ICU = append(tmplTIK.ICU, msg)
			break
		}
	}
	tmplTIK.IsComplete = !tmplTIK.IsIncomplete &&
		!tmplTIK.IsUntranslated &&
		!tmplTIK.IsInvalid
	return tmplTIK
}

// buildFilteredDataIndex builds a DataIndex with server-side filtering
// and pagination. Only TIKs within the current page get full ICU
// validation; the rest are just counted for filter stats.
//
// When searchQuery is non-empty, filter stats are skipped and results
// are fetched directly from the FTS5 index with LIMIT/OFFSET.
func (a *App) buildFilteredDataIndex(
	filterType string, showLocales, showDomains map[string]bool,
	pageIdx, pageSize int, searchQuery string,
) template.DataIndex {
	data := template.DataIndex{
		Dir:             a.dir,
		ShownLocales:    showLocales,
		ShownDomains:    showDomains,
		FilterType:      filterType,
		CanApplyChanges: a.canApplyChangesLocked(),
		TotalChanges:    len(a.changed),
		PageSize:        template.NormalizePageSize(pageSize),
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
		return a.buildSearchDataIndex(data, showLocales, pageIdx, searchQuery)
	}
	return a.buildFilterDataIndex(data, showLocales, pageIdx, filterType)
}

// buildSearchDataIndex handles the FTS search path — paginated DB query,
// no full iteration over a.tiks, no filter stats.
func (a *App) buildSearchDataIndex(
	data template.DataIndex, showLocales map[string]bool,
	pageIdx int, searchQuery string,
) template.DataIndex {
	if pageIdx < 0 {
		pageIdx = 0
	}

	result, err := a.indexDB.SearchTIKs(
		searchQuery, pageIdx*data.PageSize, data.PageSize,
	)
	if err != nil {
		return data
	}

	data.TotalFiltered = result.Total
	totalPages := data.TotalPages()
	if totalPages > 0 && pageIdx >= totalPages {
		pageIdx = totalPages - 1
	}
	data.PageIdx = pageIdx

	for _, id := range result.TIKIDs {
		if tk := a.tiksByID[id]; tk != nil {
			data.TIKs = append(data.TIKs, a.buildTIKForDisplay(tk, showLocales))
		}
	}

	return data
}

// buildFilterDataIndex handles the non-search path — full iteration
// over a.tiks with filter stats and pagination.
func (a *App) buildFilterDataIndex(
	data template.DataIndex, showLocales map[string]bool,
	pageIdx int, filterType string,
) template.DataIndex {
	filtered := make([]int, 0, len(a.tiks))
	for i, tk := range a.tiks {
		// Domain filter: skip TIKs not in any shown domain.
		if data.ShownDomains != nil && !data.ShownDomains[tk.Domain] {
			continue
		}
		hasChanged, hasUntranslated, hasIncomplete, hasInvalid := a.tikStatusFlags(tk, showLocales)
		isComplete := !hasIncomplete && !hasUntranslated && !hasInvalid

		data.NumAll++
		if isComplete {
			data.NumComplete++
		}
		if hasIncomplete {
			data.NumIncomplete++
		}
		if hasUntranslated {
			data.NumUntranslated++
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
		case "untranslated":
			if !hasUntranslated {
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

	// Clamp page index.
	if pageIdx < 0 {
		pageIdx = 0
	}
	totalPages := data.TotalPages()
	if totalPages > 0 && pageIdx >= totalPages {
		pageIdx = totalPages - 1
	}
	data.PageIdx = pageIdx

	start := pageIdx * data.PageSize
	end := min(start+data.PageSize, len(filtered))

	// Build full TIKs only for the current page.
	for _, idx := range filtered[start:end] {
		tk := a.buildTIKForDisplay(a.tiks[idx], showLocales)
		data.TIKs = append(data.TIKs, tk)
	}

	return data
}

// tikStatusFlags computes status flags for a TIK without building the
// full display data. Used for fast counting in the first pass.
func (a *App) tikStatusFlags(
	tk *template.TIK, showLocales map[string]bool,
) (hasChanged, hasUntranslated, hasIncomplete, hasInvalid bool) {
	for _, m := range tk.ICU {
		if showLocales != nil {
			if shown, ok := showLocales[m.Catalog.Locale]; !ok || !shown {
				continue
			}
		}
		if m.Message == "" {
			hasUntranslated = true
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

func (a *App) buildDomainData() template.DataDomains {
	data := template.DataDomains{
		Dir:             a.dir,
		TotalChanges:    len(a.changed),
		CanApplyChanges: a.canApplyChangesLocked(),
	}

	if a.domains == nil {
		return data
	}

	statsByName := make(map[string]*domainStats)

	for _, tk := range a.tiks {
		if tk.Domain == "" {
			continue
		}
		ds := statsByName[tk.Domain]
		if ds == nil {
			ds = &domainStats{}
			statsByName[tk.Domain] = ds
		}
		ds.numTIKs++
		hasChanged, hasUntranslated, hasIncomplete, hasInvalid := a.tikStatusFlags(tk, nil)
		if hasChanged {
			ds.changed++
		}
		if !hasIncomplete && !hasUntranslated && !hasInvalid {
			ds.complete++
		}
		if hasIncomplete {
			ds.incomplete++
		}
		if hasUntranslated {
			ds.untranslated++
		}
		if hasInvalid {
			ds.invalid++
		}
	}

	// Find root domains (no parent).
	for d := range a.domains.All() {
		if d.Parent == nil {
			data.Domains = append(data.Domains, a.buildDomainInfo(d, statsByName))
		}
	}

	data.TotalDomains = a.domains.Len()
	return data
}

// buildDomainInfo recursively builds a DomainInfo tree for d, pulling
// counts from stats keyed by each domain's fully-qualified name.
func (a *App) buildDomainInfo(
	d *codeparse.Domain, stats map[string]*domainStats,
) template.DomainInfo {
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
	if ds := stats[fullName]; ds != nil {
		info.NumTIKs = ds.numTIKs
		info.NumComplete = ds.complete
		info.NumIncomplete = ds.incomplete
		info.NumUntranslated = ds.untranslated
		info.NumInvalid = ds.invalid
		info.NumChanged = ds.changed
		if ds.numTIKs > 0 {
			info.Completeness = float64(ds.complete) / float64(ds.numTIKs)
		}
	}
	for _, sub := range d.SubDomains {
		info.SubDomains = append(info.SubDomains, a.buildDomainInfo(sub, stats))
	}
	return info
}
