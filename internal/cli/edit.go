package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/romshark/toki/editor"
	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/log"
	"github.com/romshark/toki/internal/tokibundle"
)

// ErrDesktopUnavailable is the error of `toki edit` without -server in a
// build without cgo, which the desktop app needs.
var ErrDesktopUnavailable = errors.New(
	"this build of toki was compiled without cgo; " +
		"the native desktop app is unavailable — " +
		"run `toki edit -server <addr>` instead",
)

// Edit implements the command `toki edit`.
type Edit struct{}

// Run runs the command of osArgs, logging in the language of tr. The desktop
// app speaks it too, unless the user picks another language.
func (e *Edit) Run(
	osArgs, env []string, tr tokibundle.Reader, stderr, stdout io.Writer,
) error {
	fs := newFlagSet("edit")
	// The usage of a flag.
	server := fs.String("server", "", tr.String(
		"run as plain HTTP server on this address instead of the native desktop app"))
	// The usage of a flag.
	bundlePkg := fs.String("b", config.DefaultBundlePkgPath, tr.String(
		"path to generated Go bundle package"))
	if err := flagError(tr, fs, stdout, fs.Parse(osArgs[2:])); err != nil {
		return err
	}

	log.SetWriter(stderr, false)

	dir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	a, s, err := editor.Setup(dir, *bundlePkg, Version, env,
		CleanGenerated, GenerateBundle, ApplyChangesAndBuild, RepairBundle,
		RegenerateBundle)
	if err != nil {
		return fmt.Errorf("setting up editor: %w", err)
	}

	if *server != "" {
		// Log message of toki edit -server, followed by the address it serves on.
		log.Info(tr.String("listening"), slog.String("host", *server))
		os.Exit(editor.RunServer(s, *server))
		return nil
	}

	a.SystemReader = tr
	return RunHybridApp(a, s)
}
