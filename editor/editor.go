package editor

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
	"github.com/romshark/toki/editor/app"
	"github.com/romshark/toki/editor/datapagesgen"
	"github.com/romshark/toki/editor/indexdb"
	tokisqinn "github.com/romshark/toki/editor/sqinn"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/log"
)

type noopMetrics struct{}

func (noopMetrics) OnPublish(string)   {}
func (noopMetrics) OnDeliveryDropped() {}

// CleanGeneratedFunc deletes stale generated Go files and creates a minimal bundle.
type CleanGeneratedFunc func(bundlePkgPath string) error

// GenerateBundleFunc generates Go code from a scan.
type GenerateBundleFunc func(bundlePkgPath string, scan *codeparse.Scan) error

// Setup creates the App and datapages Server for the given directory.
func Setup(
	dir, bundlePkgPath, version string, env []string,
	cleanGenerated CleanGeneratedFunc,
	generateBundle GenerateBundleFunc,
) (*app.App, *datapagesgen.Server) {
	// Extract the custom sqinn binary (built with FTS5 support).
	sqinnPath, err := tokisqinn.Path()
	if err != nil {
		log.Warn("custom sqinn not available, using default", err)
	}

	// Open or create the index database.
	var db *indexdb.DB
	if dir != "" {
		dbPath := filepath.Join(dir, ".toki", "index.db")
		var err error
		db, err = indexdb.Open(dbPath, sqinnPath)
		if err != nil {
			log.Error("opening index database", err)
		}
	}

	a := app.NewApp(dir, bundlePkgPath, env, db)
	a.Version = version
	a.SqinnPath = sqinnPath
	a.CleanGenerated = cleanGenerated
	a.GenerateGoBundle = generateBundle

	msgBroker := inmem.New(8)

	// Let the App publish EventUpdated so background builds
	// can notify SSE streams without holding an HTTP request.
	a.NotifyUpdated = func() {
		j, _ := json.Marshal(app.EventUpdated{})
		_ = msgBroker.Publish(
			context.Background(),
			noopMetrics{},
			datapagesgen.EvSubjUpdated,
			j,
		)
	}

	// Start initialization asynchronously so the server can show
	// a loading screen while the index DB is being rebuilt.
	a.SetLoading(true)
	go func() {
		defer a.SetLoading(false)
		_ = a.TryInit()
	}()

	s := datapagesgen.NewServer(a, msgBroker,
		datapagesgen.WithAssets(app.StaticFS),
	)
	s.UseContextCanceledFilter()

	// Register the repair handler for fixing corrupt native locale messages.
	// Repairs are applied as changes (preserving existing unsaved edits)
	// and redirect back to the dashboard.
	s.Mux().HandleFunc("GET /repair/{$}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.RepairCorrupt(); err != nil {
			http.Error(w, "Repair failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

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
