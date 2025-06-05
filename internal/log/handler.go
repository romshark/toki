package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/fatih/color"
)

type colorHandler struct {
	w     io.Writer
	mu    sync.Mutex
	level slog.Leveler
}

func newColorHandler(w io.Writer, lvl slog.Leveler) *colorHandler {
	return &colorHandler{level: lvl, w: w}
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

var (
	colorLevelErr     = color.New(color.BgRed, color.FgHiRed, color.Bold)
	colorLevelWarn    = color.New(color.FgYellow)
	colorLevelInfo    = color.New(color.FgGreen)
	colorLevelDebug   = color.New(color.FgCyan)
	colorFaint        = color.New(color.Faint)
	colorFieldName    = color.New(color.FgHiWhite)
	colorFieldVal     = color.New(color.FgHiBlue)
	colorFieldValNum  = color.New(color.FgHiMagenta)
	colorFieldValBool = color.New(color.FgHiGreen)
)

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	levelColor := colorFaint
	switch {
	case r.Level >= slog.LevelError:
		levelColor = colorLevelErr
	case r.Level >= slog.LevelWarn:
		levelColor = colorLevelWarn
	case r.Level >= slog.LevelInfo:
		levelColor = colorLevelInfo
	case r.Level >= slog.LevelDebug:
		levelColor = colorLevelDebug
	}

	var b strings.Builder
	_, _ = levelColor.Fprintf(&b, "[%s]", strings.ToUpper(r.Level.String()))
	b.WriteString(" ")
	b.WriteString(r.Message)

	attrs := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		key, val := a.Key, formatAttrValue(a.Value)
		attrs[key] = val
		return true
	})

	// Sort keys for output consistency.
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if len(keys) > 0 {
		b.WriteString("\n")
	}
	for i, k := range keys {
		_, _ = colorFieldName.Fprint(&b, k)
		_, _ = colorFaint.Fprint(&b, ": ")
		_, _ = colorFieldName.Fprint(&b, attrs[k])
		if i+1 < len(keys) {
			b.WriteString("\n")
		}
	}

	// Print.
	h.mu.Lock()
	defer h.mu.Unlock()

	_, _ = fmt.Fprintln(h.w, b.String())
	return nil
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *colorHandler) WithGroup(name string) slog.Handler       { return h }

func formatAttrValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return colorFieldVal.Sprintf("%q", v.String())
	case slog.KindInt64:
		return colorFieldValNum.Sprintf("%d", v.Int64())
	case slog.KindFloat64:
		return colorFieldValNum.Sprintf("%f", v.Float64())
	case slog.KindBool:
		return colorFieldValBool.Sprintf("%t", v.Bool())
	default:
		return fmt.Sprintf("%v", v.Any())
	}
}
