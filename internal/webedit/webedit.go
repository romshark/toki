package webedit

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"

	"golang.org/x/text/language"

	"github.com/romshark/icumsg"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/icu"
	"github.com/romshark/toki/internal/log"
	"github.com/romshark/toki/internal/webedit/template"
)

type Server struct {
	httpServer *http.Server

	newScan func() (*codeparse.Scan, error)

	lock         sync.Mutex
	scan         *codeparse.Scan
	icuTokenizer *icumsg.Tokenizer
	icuTokBuffer []icumsg.Token
	tiks         []*template.TIK
	catalogs     []*template.Catalog
	localeTags   []language.Tag
	changed      []*template.ICUMessage
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func NewServer(host string, newScan func() (*codeparse.Scan, error)) *Server {
	s := &Server{
		newScan:      newScan,
		icuTokenizer: new(icumsg.Tokenizer),
		httpServer: &http.Server{
			Addr: host,
		},
	}

	// the files in staticFS are still under the "static" directory.
	// strip it so the file server can serve "/static/something.js"
	staticFiles, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}

	m := http.NewServeMux()
	m.Handle("GET /favicon.ico", http.HandlerFunc(s.handleGetFavicon))
	m.Handle("GET /static/",
		http.StripPrefix("/static/",
			http.FileServer(http.FS(staticFiles)),
		))
	m.Handle("GET /", http.HandlerFunc(s.handleGetIndex))
	m.Handle("POST /set", http.HandlerFunc(s.handlePostSet))
	m.Handle("POST /apply-changes", http.HandlerFunc(s.handlePostApplyChanges))
	s.httpServer.Handler = m

	return s
}

func (s *Server) Init() (err error) {
	s.changed = nil // Reset all changes.
	s.scan, err = s.newScan()
	if err != nil {
		return err
	}
	s.localeTags = make([]language.Tag, 0, len(s.catalogs))
	s.catalogs = make([]*template.Catalog, 0, s.scan.Catalogs.Len())
	s.tiks = make([]*template.TIK, 0, s.scan.TextIndexByID.Len())
	for cat := range s.scan.Catalogs.SeqRead() {
		c := &template.Catalog{
			Locale:  cat.ARB.Locale.String(),
			Default: cat.ARB.Locale == s.scan.DefaultLocale,
		}
		s.catalogs = append(s.catalogs, c)
		s.localeTags = append(s.localeTags, language.MustParse(c.Locale))
	}

	for _, i := range s.scan.TextIndexByID.SeqRead() {
		t := s.scan.Texts.At(i)
		tmplTIK := &template.TIK{
			ID:          t.IDHash,
			TIK:         t.TIK.Raw,
			Description: strings.Join(t.Comments, " "),
			ICU:         make([]*template.ICUMessage, 0, len(s.catalogs)),
		}
		for c := range s.scan.Catalogs.SeqRead() {
			m := c.ARB.Messages[t.IDHash]
			tmplMsg := &template.ICUMessage{
				ID: m.ID,
				Catalog: func() *template.Catalog {
					for i, c2 := range s.catalogs {
						if c.ARB.Locale == s.localeTags[i] {
							return c2
						}
					}
					return nil
				}(),
				Message: m.ICUMessage,
			}
			tmplTIK.ICU = append(tmplTIK.ICU, tmplMsg)
		}
		s.tiks = append(s.tiks, tmplTIK)
	}
	sort.Slice(s.tiks, func(i, j int) bool {
		return s.tiks[i].ID < s.tiks[j].ID
	})
	return nil
}

//go:embed static/favicon.ico
var faviconICO []byte

//go:embed static/*
var staticFS embed.FS

func (s *Server) handleGetFavicon(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write(faviconICO)
}

func (s *Server) handleGetIndex(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	noCacheHeaders(w)

	if r.URL.Path != "/" {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	hideLocales, filterTIKs, ok := parseFilterParams(w, r)
	if !ok {
		return
	}

	data := s.newDataIndex(hideLocales, filterTIKs)
	if r.Header.Get("Hx-Request") == "true" {
		template.RenderViewIndex(w, r, data)
		return
	}

	template.RenderPageIndex(w, r, data)
}

// func debugForm(w http.ResponseWriter, r *http.Request) {
// 	if err := r.ParseForm(); err != nil {
// 		http.Error(w, "cannot parse form: "+err.Error(), http.StatusBadRequest)
// 		return
// 	}
// 	for k, vs := range r.Form {
// 		fmt.Fprintf(os.Stdout, "%q = %q\n", k, vs)
// 	}
// }

func (s *Server) handlePostSet(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	noCacheHeaders(w)

	// Parse form data.
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	id := r.FormValue("id")
	locale := r.FormValue("locale")
	newMessage := r.FormValue("icumsg")

	if id == "" || locale == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	iCatalog := slices.IndexFunc(s.catalogs, func(c *template.Catalog) bool {
		return c.Locale == locale
	})
	if iCatalog == -1 {
		http.Error(w, "no catalog for locale", http.StatusBadRequest)
		return
	}
	iTIK := slices.IndexFunc(s.tiks, func(t *template.TIK) bool {
		return t.ID == id
	})
	if iTIK == -1 {
		http.Error(w, "TIK not found", http.StatusBadRequest)
		return
	}
	tk := s.tiks[iTIK]

	iICUMsg := slices.IndexFunc(tk.ICU, func(m *template.ICUMessage) bool {
		return m.Catalog == s.catalogs[iCatalog]
	})
	icuMsg := tk.ICU[iICUMsg]

	if icuMsg.Message == newMessage {
		template.RenderFragmentICUMessage(w, r, id, icuMsg)
		return // No change.
	}

	loc := s.localeTags[iCatalog]

	var err error
	s.icuTokBuffer = s.icuTokBuffer[:0]
	s.icuTokBuffer, err = s.icuTokenizer.Tokenize(
		loc, s.icuTokBuffer, newMessage,
	)
	if err != nil {
		icuMsg.Error = fmt.Sprintf("at index %d: %v", s.icuTokenizer.Pos(), err)
	} else {
		icuMsg.IncompleteReports = icu.AnalysisReport(
			loc, newMessage, s.icuTokBuffer, codeparse.ICUSelectOptions,
		)
	}

	if icuMsg.Changed {
		if newMessage == icuMsg.MessageOriginal {
			// Reverted change.
			icuMsg.Message = newMessage
			icuMsg.Changed = false
			icuMsg.MessageOriginal = ""
			s.changed = slices.DeleteFunc(s.changed, func(m *template.ICUMessage) bool {
				return m == icuMsg
			})
		} else {
			// Changed repeatedly.
			icuMsg.Message = newMessage
		}
	} else {
		// First time change.
		icuMsg.Changed = true
		icuMsg.MessageOriginal = icuMsg.Message
		icuMsg.Message = newMessage
		s.changed = append(s.changed, icuMsg)
	}

	hideLocales, filterTIKs, ok := parseFilterParamsFromReferer(w, r)
	if !ok {
		return
	}
	template.RenderOOBUpdate(w, r, id, icuMsg, s.newDataIndex(hideLocales, filterTIKs))
}

func parseFilterParams(
	w http.ResponseWriter, r *http.Request,
) ([]string, template.FilterTIKs, bool) {
	q := r.URL.Query()

	filterTIKs, ok := parseFilterTIKs(w, q)
	if !ok {
		return nil, 0, false
	}

	hideLocales := q["hl"]
	return hideLocales, filterTIKs, true
}

func parseFilterParamsFromReferer(
	w http.ResponseWriter, r *http.Request,
) ([]string, template.FilterTIKs, bool) {
	referer := r.Header.Get("Referer")
	if referer == "" {
		// Fallback to default if no referer
		return nil, template.FilterTIKsAll, true
	}

	refererURL, err := url.Parse(referer)
	if err != nil {
		return nil, template.FilterTIKsAll, true
	}

	q := refererURL.Query()

	filterTIKs, ok := parseFilterTIKs(w, q)
	if !ok {
		filterTIKs = template.FilterTIKsAll
	}

	hideLocales := q["hl"]
	return hideLocales, filterTIKs, true
}

func parseFilterTIKs(
	w http.ResponseWriter, q url.Values,
) (tp template.FilterTIKs, ok bool) {
	switch q.Get("t") {
	case "all", "":
		tp = template.FilterTIKsAll
	case "changed":
		tp = template.FilterTIKsChanged
	case "empty":
		tp = template.FilterTIKsEmpty
	case "complete":
		tp = template.FilterTIKsComplete
	case "incomplete":
		tp = template.FilterTIKsIncomplete
	case "invalid":
		tp = template.FilterTIKsInvalid
	default:
		http.Error(w, "invalid type", http.StatusNotAcceptable)
		return 0, false
	}
	return tp, true
}

func (s *Server) newDataIndex(
	hideCatalogLocales []string,
	filterType template.FilterTIKs,
) template.DataIndex {
	isCatalogHidden := func(locale string) bool {
		if hideCatalogLocales == nil {
			return false
		}
		return slices.Contains(hideCatalogLocales, locale)
	}

	data := template.DataIndex{
		Catalogs:          s.catalogs,
		FilterTIKs:        filterType,
		CatalogsDisplayed: make([]*template.Catalog, 0, len(hideCatalogLocales)),
		CanApplyChanges:   s.canApplyChanges(),
	}

	// Move default catalog to first index.
	for i, c := range data.Catalogs {
		if c.Default {
			data.Catalogs[0], data.Catalogs[i] = data.Catalogs[i], data.Catalogs[0]
			break
		}
	}

	for _, cat := range s.catalogs {
		if !isCatalogHidden(cat.Locale) {
			data.CatalogsDisplayed = append(data.CatalogsDisplayed, cat)
		}
	}

	locales := make([]language.Tag, len(s.catalogs))
	for i, c := range s.catalogs {
		locales[i] = language.MustParse(c.Locale)
	}

	var tiks []template.TIK
	for _, tk := range s.tiks {
		tmplTIK := template.TIK{
			ID:          tk.ID,
			TIK:         tk.TIK,
			Description: tk.Description,
			ICU:         make([]*template.ICUMessage, 0, len(s.catalogs)),
		}
		isInvalid := false
		isIncomplete := false
		isEmpty := false
		isChanged := false
		for catIndex, c := range s.catalogs {
			if isCatalogHidden(c.Locale) {
				continue
			}

			m := func() *template.ICUMessage {
				for _, iTIK := range tk.ICU {
					if iTIK.Catalog == c {
						return iTIK
					}
				}
				return nil
			}()

			var err error
			s.icuTokBuffer = s.icuTokBuffer[:0]
			s.icuTokBuffer, err = s.icuTokenizer.Tokenize(
				s.localeTags[catIndex], s.icuTokBuffer, m.Message,
			)
			if err != nil {
				isInvalid = true
				m.Error = fmt.Sprintf("at index %d: %v", s.icuTokenizer.Pos(), err)
			}

			m.IncompleteReports = icu.AnalysisReport(
				s.localeTags[catIndex], m.Message, s.icuTokBuffer,
				codeparse.ICUSelectOptions,
			)
			if len(m.IncompleteReports) != 0 {
				isIncomplete = true
			}

			if m.Message == "" {
				isEmpty = true
				isIncomplete = true
			}

			if m.Changed {
				isChanged = true
			}

			tmplTIK.ICU = append(tmplTIK.ICU, m)
		}
		data.NumAll++
		if !isIncomplete {
			data.NumComplete++
		}
		if isIncomplete {
			data.NumIncomplete++
		}
		if isEmpty {
			data.NumEmpty++
		}
		if isInvalid {
			data.NumInvalid++
		}
		if isChanged {
			data.NumChanged++
		}
		data.TotalChanges = len(s.changed)

		switch filterType {
		case template.FilterTIKsComplete:
			if !isIncomplete {
				tiks = append(tiks, tmplTIK)
			}
		case template.FilterTIKsChanged:
			if isChanged {
				tiks = append(tiks, tmplTIK)
			}
		case template.FilterTIKsIncomplete:
			if isIncomplete {
				tiks = append(tiks, tmplTIK)
			}
		case template.FilterTIKsEmpty:
			if isEmpty {
				tiks = append(tiks, tmplTIK)
			}
		case template.FilterTIKsInvalid:
			if isInvalid {
				tiks = append(tiks, tmplTIK)
			}
		default:
			tiks = append(tiks, tmplTIK)
		}
	}
	data.TIKs = func(yield func(template.TIK) bool) {
		for _, t := range tiks {
			if !yield(t) {
				break
			}
		}
	}
	return data
}

func (s *Server) canApplyChanges() bool {
	for _, c := range s.changed {
		if c.Error != "" {
			return false
		}
	}
	return true
}

func (s *Server) handlePostApplyChanges(w http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.canApplyChanges() {
		http.Error(w, "can't apply changes", http.StatusBadRequest)
		return
	}

	type ARBFile struct {
		*arb.File
		AbsolutePath string
		Changed      bool
	}

	arbFiles := make(map[string]ARBFile, s.scan.Catalogs.Len())
	for c := range s.scan.Catalogs.Seq() {
		arbFiles[c.ARB.Locale.String()] = ARBFile{
			File:         c.ARB,
			AbsolutePath: c.ARBFilePath,
		}
	}

	for _, c := range s.changed {
		log.Info("apply change",
			slog.String("id", c.ID),
			slog.String("catalog", c.Catalog.Locale),
			slog.String("new", c.Message),
			slog.String("original", c.MessageOriginal))

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
		filePath := arbFile.AbsolutePath

		err := func() error {
			log.Info("writing changed catalog",
				slog.String("file", filePath))

			f, err := os.OpenFile(filePath, os.O_WRONLY, 0o644)
			if err != nil {
				return fmt.Errorf("opening arb file: %w", err)
			}

			defer func() {
				if err := f.Close(); err != nil {
					log.Error("closing arb file", err, slog.String("file", filePath))
				}
			}()

			err = arb.Encode(f, arbFile.File, "\t")
			if err != nil {
				return fmt.Errorf("encoding arb file: %w", err)
			}
			return nil
		}()
		if err != nil {
			log.Error("writing changed catalog", err, slog.String("file", filePath))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Re-initialize server.
	if err := s.Init(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusNoContent)
}

func noCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}
