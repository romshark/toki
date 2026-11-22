package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/gengo"
	"github.com/romshark/toki/internal/log"
	"github.com/romshark/toki/internal/tokibundle"
)

type Result struct {
	Config     *config.ConfigGenerate
	Start      time.Time
	Scan       *codeparse.Scan
	NewTexts   []codeparse.Text
	RemovedIDs []string // IDs of the TIKs removed from the source.
	Err        error
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

func (r Result) mustPrintJSON(stdout io.Writer) {
	enc := json.NewEncoder(stdout)
	var errMsg string
	if r.Err != nil {
		errMsg = r.Err.Error()
	}
	data := ResultJSON{
		Error:   errMsg,
		TIKsNew: len(r.NewTexts),
		TimeMS:  time.Since(r.Start).Milliseconds(),
	}
	// Errors that stop the source scan leave no scan to report.
	if r.Scan != nil {
		data.StringCalls = r.Scan.StringCalls.Load()
		data.WriteCalls = r.Scan.WriteCalls.Load()
		data.TIKs = r.Scan.Texts.Len()
		data.TIKsUnique = r.Scan.TextIndexByID.Len()
		data.FilesTraversed = int(r.Scan.FilesTraversed.Load())
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
	}
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		panic(fmt.Errorf("encoding JSON to stdout: %w", err))
	}
}

// logReader returns the reader of what a command with conf logs: tr, or
// English in JSON mode, whose output programs read. conf is nil for the
// commands without -json.
func logReader(tr tokibundle.Reader, conf *config.ConfigGenerate) tokibundle.Reader {
	if conf != nil && conf.JSON {
		return tokibundle.Default()
	}
	return tr
}

// Print logs r in the language of tr and, in JSON mode, writes it to stdout
// as JSON.
func (r Result) Print(tr tokibundle.Reader, stdout io.Writer) {
	tr = logReader(tr, r.Config)
	if r.Err != nil {
		log.Error(errorText(tr, r.Err), nil)
	}
	if r.Config == nil {
		return
	}
	if r.Config.JSON {
		r.mustPrintJSON(stdout)
		return
	}

	if r.Scan != nil {
		_ = r.Scan.SourceErrors.Access(func(s []codeparse.SourceError) error {
			if l := len(s); l > 0 {
				// Log message that heads the list of the errors in the source code.
				log.Error(tr.String("source errors"), nil, slog.Int("total", l))
				for _, e := range s {
					// Log message of an error in the source code, which follows
					// in English with its position.
					log.Error(tr.String("source"), e.Err,
						slog.String("pos", log.FmtPos(e.Position)))
				}
			}
			return nil
		})

		fields := []any{
			slog.Int("tiks.total", r.Scan.Texts.Len()),
			slog.Int("tiks.unique", r.Scan.TextIndexByID.Len()),
			slog.Int("tiks.new", len(r.NewTexts)),
			slog.Int("tiks.removed", len(r.RemovedIDs)),
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
		// Log message of the report at the end of toki generate and toki lint.
		log.Info(tr.String("finished"), fields...)
	}
}
