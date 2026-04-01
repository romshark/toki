package datapagesgen

import (
	"context"
	"log/slog"
	"net/http"
)

// Mux returns the server's HTTP mux for registering custom routes.
func (s *Server) Mux() *http.ServeMux { return s.mux }

// SetAssetsDir overrides the static assets directory with an absolute path.
func (s *Server) SetAssetsDir(dir string) {
	s.assetsFS = http.Dir(dir)
}

// UseContextCanceledFilter wraps the server's logger to suppress
// context.Canceled errors, which are normal during SSE request cancellation.
func (s *Server) UseContextCanceledFilter() {
	s.logger = slog.New(&contextCanceledFilter{inner: s.logger.Handler()})
}

type contextCanceledFilter struct {
	inner slog.Handler
}

func (h *contextCanceledFilter) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *contextCanceledFilter) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == slog.LevelError {
		var hasCtxCanceled bool
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "err" {
				if err, ok := a.Value.Any().(error); ok {
					if context.Cause(ctx) == context.Canceled ||
						err == context.Canceled ||
						err.Error() == "failed to patch element: context canceled" {
						hasCtxCanceled = true
						return false
					}
				}
			}
			return true
		})
		if hasCtxCanceled {
			return nil
		}
	}
	return h.inner.Handle(ctx, r)
}

func (h *contextCanceledFilter) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextCanceledFilter{inner: h.inner.WithAttrs(attrs)}
}

func (h *contextCanceledFilter) WithGroup(name string) slog.Handler {
	return &contextCanceledFilter{inner: h.inner.WithGroup(name)}
}
