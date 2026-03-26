package editor

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
	"github.com/romshark/toki/editor/app"
	"github.com/romshark/toki/editor/datapagesgen"
	"github.com/romshark/toki/internal/log"
)

// Setup creates the App and datapages Server for the given directory.
func Setup(dir, bundlePkgPath string, env []string) (*app.App, *datapagesgen.Server) {
	a := app.NewApp(dir, bundlePkgPath, env)
	_ = a.TryInit()

	msgBroker := inmem.New(8)
	s := datapagesgen.NewServer(a, msgBroker,
		datapagesgen.WithAssets(app.StaticFS),
	)
	return a, s
}

// RunServer starts the HTTP server on the given address and blocks until interrupted.
func RunServer(s *datapagesgen.Server, host string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Info("listening", slog.String("host", host))
	err := s.ListenAndServe(ctx, host)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("listening and serving HTTP", err)
		return 1
	}
	return 0
}
