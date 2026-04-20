package editor

import (
	"context"
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
	"golang.org/x/text/language"
)

// CleanGeneratedFunc deletes stale generated Go files and creates a minimal bundle.
type CleanGeneratedFunc func(bundlePkgPath string, defaultLocale language.Tag) error

// GenerateBundleFunc generates Go code from a scan.
type GenerateBundleFunc func(bundlePkgPath string, scan *codeparse.Scan) error

// ApplyChangesAndBuildFunc persists the supplied ARB edits and regenerates
// the Go bundle, returning the post-build scan.
type ApplyChangesAndBuildFunc func(
	env []string, modDir, bundlePkgPath string, defaultLocale language.Tag,
	edits []app.BundleEdit,
) (*codeparse.Scan, error)

// RepairBundleFunc fixes corrupt native-locale entries by regenerating them
// from TIK source. Returns the number of messages repaired (0 if the bundle
// is already consistent).
type RepairBundleFunc func(env []string, modDir, bundlePkgPath string) (int, error)

// RegenerateBundleFunc runs `toki generate` programmatically — adds missing
// native ARB entries and regenerates the Go bundle.
type RegenerateBundleFunc func(env []string, modDir, bundlePkgPath string) error

// Setup creates the App and datapages Server for the given directory.
func Setup(
	dir, bundlePkgPath, version string, env []string,
	cleanGenerated CleanGeneratedFunc,
	generateBundle GenerateBundleFunc,
	applyChangesAndBuild ApplyChangesAndBuildFunc,
	repairBundle RepairBundleFunc,
	regenerateBundle RegenerateBundleFunc,
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
	a.ApplyChangesAndBuild = applyChangesAndBuild
	a.RepairBundle = repairBundle
	a.RegenerateBundle = regenerateBundle

	// Start initialization asynchronously so the server can show
	// a loading screen while the index DB is being rebuilt.
	a.SetLoading(true)
	go func() {
		defer a.SetLoading(false)
		_ = a.TryInit()
	}()

	s := datapagesgen.NewServer(a, inmem.New(8),
		datapagesgen.WithAssets(app.StaticFS),
	)
	s.UseContextCanceledFilter()

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
