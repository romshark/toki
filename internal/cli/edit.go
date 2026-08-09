package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/romshark/toki/editor"
	"github.com/romshark/toki/internal/log"
)

// Edit implements the command `toki edit`.
type Edit struct{}

func (e *Edit) Run(osArgs, env []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", "",
		"run as plain HTTP server on this address instead of the native desktop app")
	bundlePkg := fs.String("b", "tokibundle", "path to generated Go bundle package")
	if err := fs.Parse(osArgs[2:]); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCLIArgs, err)
	}

	log.SetWriter(stderr, false)

	dir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	a, s := editor.Setup(dir, *bundlePkg, Version, env,
		CleanGenerated, GenerateBundle, ApplyChangesAndBuild, RepairBundle,
		RegenerateBundle)

	if *server != "" {
		os.Exit(editor.RunServer(s, *server))
		return nil
	}

	return RunHybridApp(a, s)
}
