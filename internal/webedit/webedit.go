package webedit

import (
	"embed"
	"io/fs"
	"iter"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/romshark/icumsg"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/icu"
	"github.com/romshark/toki/internal/webedit/template"
)

func NewServer(host string, s *codeparse.Scan) *http.Server {
	// the files in staticFS are still under the "static" directory.
	// strip it so the file server can serve "/static/something.js"
	staticFiles, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}

	h := &handler{
		state: newState(s),
	}
	m := http.NewServeMux()
	m.Handle("GET /favicon.ico", http.HandlerFunc(h.handleGetFavicon))
	m.Handle("GET /static/",
		http.StripPrefix("/static/",
			http.FileServer(http.FS(staticFiles)),
		))
	m.Handle("GET /", http.HandlerFunc(h.handleGetIndex))
	m.Handle("POST /set", http.HandlerFunc(h.handlePostSet))
	return &http.Server{
		Addr:    host,
		Handler: m,
	}
}

type handler struct {
	state *state
}

//go:embed static/favicon.ico
var faviconICO []byte

//go:embed static/*
var staticFS embed.FS

func (h *handler) handleGetFavicon(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write(faviconICO)
}

func (h *handler) handleGetIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filterTIKs, ok := parseFilterTIKs(w, q)
	if !ok {
		return
	}

	hideLocales := q["hl"]

	h.state.Read(func(s *state) {
		stats, iterTIKs := s.QueryTIKs(filterTIKs, hideLocales)

		dataIndex := template.DataIndex{
			NumChanges:        0,
			NumAll:            stats.NumAll,
			NumEmpty:          stats.NumEmpty,
			NumComplete:       stats.NumComplete,
			NumIncomplete:     stats.NumIncomplete,
			NumInvalid:        stats.NumInvalid,
			TIKs:              iterTIKs,
			Catalogs:          s.Catalogs,
			CatalogsDisplayed: s.Catalogs,
			FilterTIKs:        filterTIKs,
		}

		if len(hideLocales) != 0 {
			dataIndex.CatalogsDisplayed = make([]*template.Catalog, 0, len(s.Catalogs))
			for _, c := range s.Catalogs {
				if c.Default || !slices.Contains(hideLocales, c.Locale) {
					dataIndex.CatalogsDisplayed = append(dataIndex.CatalogsDisplayed, c)
				}
			}
		}

		if r.Header.Get("Hx-Request") == "true" {
			template.RenderViewIndex(w, r, dataIndex)
			return
		}

		template.RenderPageIndex(w, r, dataIndex)
	})
}

func (h *handler) handlePostSet(w http.ResponseWriter, r *http.Request) {
}

func parseFilterTIKs(w http.ResponseWriter, q url.Values) (tp template.FilterTIKs, ok bool) {
	switch q.Get("t") {
	case "all", "":
		tp = template.FilterTIKsAll
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

type Statistics struct {
	NumAll        int
	NumEmpty      int
	NumComplete   int
	NumIncomplete int
	NumInvalid    int
}

type state struct {
	lock       sync.RWMutex
	Catalogs   []*template.Catalog
	All        []*template.TIK
	Empty      []*template.TIK
	Complete   []*template.TIK
	Incomplete []*template.TIK
	Invalid    []*template.TIK
}

func (s *state) Read(fn func(s *state)) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	fn(s)
}

func (s *state) ReadWrite(fn func(s *state)) {
	s.lock.Lock()
	defer s.lock.Unlock()
	fn(s)
}

func newState(scan *codeparse.Scan) *state {
	icutk := new(icumsg.Tokenizer)
	var icuBuf []icumsg.Token

	s := &state{}
	for cat := range scan.Catalogs.Seq() {
		s.Catalogs = append(s.Catalogs, &template.Catalog{
			Locale:  cat.ARB.Locale.String(),
			Default: cat.ARB.Locale == scan.DefaultLocale,
		})
	}

	for _, i := range scan.TextIndexByID.Seq() {
		t := scan.Texts.At(i)
		tmpl := &template.TIK{
			ID:          t.IDHash,
			TIK:         t.TIK.Raw,
			Description: strings.Join(t.Comments, " "),
		}
		isInvalid := false
		isIncomplete := false
		isEmpty := false
		for c := range scan.Catalogs.Seq() {
			m := c.ARB.Messages[t.IDHash]

			tmplMsg := template.ICUMessage{
				Locale:  c.ARB.Locale.String(),
				Message: m.ICUMessage,
			}

			var err error
			icuBuf = icuBuf[:0]
			icuBuf, err = icutk.Tokenize(c.ARB.Locale, icuBuf, m.ICUMessage)
			if err != nil {
				isInvalid = true
				tmplMsg.Error = err.Error()
			}

			tmplMsg.IncompleteReports = icu.CompletenessReport(
				c.ARB.Locale, m.ICUMessage, icuBuf, codeparse.ICUSelectOptions,
			)
			if len(tmplMsg.IncompleteReports) != 0 {
				isIncomplete = true
			}

			for _, msg := range tmpl.ICU {
				if msg.Message == "" {
					isEmpty = true
					break
				}
			}

			tmpl.ICU = append(tmpl.ICU, tmplMsg)
		}
		s.All = append(s.All, tmpl)
		if isEmpty {
			s.Empty = append(s.Empty, tmpl)
		}
		if isIncomplete {
			s.Incomplete = append(s.Incomplete, tmpl)
		} else {
			s.Complete = append(s.Complete, tmpl)
		}
		if isInvalid {
			s.Invalid = append(s.Invalid, tmpl)
		}

	}
	return s
}

func (s *state) QueryTIKs(
	tp template.FilterTIKs, hideLocales []string,
) (Statistics, iter.Seq[template.TIK]) {
	r := []*template.TIK{}
	switch tp {
	case template.FilterTIKsAll:
		r = s.All
	case template.FilterTIKsEmpty:
		r = s.Empty
	case template.FilterTIKsComplete:
		r = s.Complete
	case template.FilterTIKsIncomplete:
		r = s.Incomplete
	case template.FilterTIKsInvalid:
		r = s.Invalid
	}
	itr := func(yield func(template.TIK) bool) {
		for _, m := range r {
			cp := *m
			if hideLocales != nil {
				// Locale filter enabled.
				cp.ICU = make([]template.ICUMessage, 0, len(m.ICU))
				for _, mICU := range m.ICU {
					if !slices.Contains(hideLocales, mICU.Locale) {
						cp.ICU = append(cp.ICU, mICU)
					}
				}
			}
			if !yield(cp) {
				break
			}
		}
	}
	return Statistics{
		NumAll:        len(s.All),
		NumEmpty:      len(s.Empty),
		NumComplete:   len(s.Complete),
		NumIncomplete: len(s.Incomplete),
		NumInvalid:    len(s.Invalid),
	}, itr
}
