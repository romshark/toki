package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/gengo"
	"github.com/romshark/toki/internal/log"
)

type Result struct {
	Config       *config.ConfigGenerate
	Start        time.Time
	Scan         *codeparse.Scan
	NewTexts     []codeparse.Text
	RemovedTexts []codeparse.Text
	Err          error
}

type ResultJSONCatalog struct {
	Locale       string  `json:"locale"`
	Completeness float64 `json:"completeness"`
}
type ResultJSONSourceError struct {
	Error string `json:"error"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

type ResultJSON struct {
	Error          string                  `json:"error,omitempty"`
	StringCalls    int64                   `json:"string-calls"`
	WriteCalls     int64                   `json:"write-calls"`
	TIKs           int                     `json:"tiks"`
	TIKsUnique     int                     `json:"tiks-unique"`
	TIKsNew        int                     `json:"tiks-new"`
	FilesTraversed int                     `json:"files-traversed"`
	SourceErrors   []ResultJSONSourceError `json:"source-errors,omitempty"`
	TimeMS         int64                   `json:"time-ms"`
	Catalogs       []ResultJSONCatalog     `json:"catalogs"`
}

func (r Result) mustPrintJSON() {
	enc := json.NewEncoder(os.Stderr)
	var errMsg string
	if r.Err != nil {
		errMsg = r.Err.Error()
	}
	data := ResultJSON{
		Error:          errMsg,
		StringCalls:    r.Scan.StringCalls.Load(),
		WriteCalls:     r.Scan.WriteCalls.Load(),
		TIKs:           r.Scan.Texts.Len(),
		TIKsUnique:     r.Scan.TextIndexByID.Len(),
		TIKsNew:        len(r.NewTexts),
		FilesTraversed: int(r.Scan.FilesTraversed.Load()),
		TimeMS:         time.Since(r.Start).Milliseconds(),
	}
	_ = r.Scan.SourceErrors.Access(func(s []codeparse.SourceError) error {
		data.SourceErrors = make([]ResultJSONSourceError, len(s))
		for i, serr := range s {
			data.SourceErrors[i] = ResultJSONSourceError{
				Error: serr.Err.Error(),
				File:  serr.Filename,
				Line:  serr.Line,
				Col:   serr.Column,
			}
		}
		return nil
	})
	_ = r.Scan.Catalogs.Access(func(s []*codeparse.Catalog) error {
		data.Catalogs = make([]ResultJSONCatalog, len(s))
		for i, c := range s {
			completeness := completeness(c)
			data.Catalogs[i] = ResultJSONCatalog{
				Locale:       c.ARB.Locale.String(),
				Completeness: completeness,
			}
		}
		return nil
	})
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		panic(fmt.Errorf("encoding JSON to stderr: %w", err))
	}
}

func (r Result) Print() {
	if r.Config == nil {
		return
	}
	if r.Config.JSON {
		r.mustPrintJSON()
		return
	}

	if r.Scan != nil {
		_ = r.Scan.SourceErrors.Access(func(s []codeparse.SourceError) error {
			if l := len(s); l > 0 {
				log.Error("source errors", nil, slog.Int("total", l))
				for _, e := range s {
					log.Error("source", e.Err, slog.String("pos", log.FmtPos(e.Position)))
				}
			}
			return nil
		})

		fields := []any{
			slog.Int("tiks.total", r.Scan.Texts.Len()),
			slog.Int("tiks.unique", r.Scan.TextIndexByID.Len()),
			slog.Int("tiks.new", len(r.NewTexts)),
			slog.Int("tiks.removed", len(r.RemovedTexts)),
			slog.Int64("scan.files", r.Scan.FilesTraversed.Load()),
			slog.String("scan.duration", time.Since(r.Start).String()),
			slog.Int64("catalogs", int64(r.Scan.Catalogs.Len())),
		}
		_ = r.Scan.Catalogs.Access(func(s []*codeparse.Catalog) error {
			for _, c := range s {
				fieldName := gengo.FileNameWithLocale(c.ARB.Locale, "catalog", ".arb")
				completeness := completeness(c) * 100
				fields = append(fields, slog.Group(fieldName,
					slog.String("completeness", fmt.Sprintf("%.2f%%", completeness))))
			}
			return nil
		})
		log.Info("finished", fields...)
	}
	if r.Err != nil {
		log.Error(r.Err.Error(), nil)
	}
}
