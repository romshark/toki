package app

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/cli/browser"
	"github.com/romshark/toki/editor"
	"github.com/romshark/toki/internal/log"
)

// Edit implements the command `toki edit`.
type Edit struct{}

func (e *Edit) Run(osArgs, env []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", "", "run as plain HTTP server on this address")
	bundlePkg := fs.String("b", "tokibundle", "path to generated Go bundle package")
	if err := fs.Parse(osArgs[2:]); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCLIArgs, err)
	}

	log.SetWriter(stderr, false)

	dir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	_, s := editor.Setup(dir, *bundlePkg, env, CleanGenerated, GenerateBundle)

	if *server != "" {
		os.Exit(editor.RunServer(s, *server))
		return nil
	}

	// Default: start on free port and open browser.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	go func() {
		// Wait for server to be ready, then open browser.
		for {
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				break
			}
		}
		_ = browser.OpenURL(fmt.Sprintf("http://%s", addr))
	}()

	os.Exit(editor.RunServer(s, addr))
	return nil
}
