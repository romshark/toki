package cli

// #cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
import "C"

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/romshark/toki/editor/app"
	"github.com/romshark/toki/editor/datapagesgen"
)

// RunHybridApp launches the editor as a native Wails desktop app backed by
// a local HTTP server on an ephemeral 127.0.0.1 port. Returns when the
// webview window closes or the server fails to start.
//
// This file is compiled only when cgo is enabled (the `import "C"` above
// acts as an implicit build constraint); edit_nocgo.go provides a stub
// used under CGO_ENABLED=0 (e.g. the govulncheck CI job).
func RunHybridApp(a *app.App, s *datapagesgen.Server) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	wailsApp := application.New(application.Options{
		Name: "Toki Editor",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: func() { serverCancel() },
	})

	a.PickDirectory = func() (string, error) {
		return wailsApp.Dialog.OpenFile().
			CanChooseDirectories(true).
			CanChooseFiles(false).
			SetTitle("Select Project Folder").
			PromptForSingleSelection()
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- s.ListenAndServe(serverCtx, addr)
	}()

	// Wait until the server is accepting connections (or fails).
	for {
		select {
		case err := <-serverErr:
			return fmt.Errorf("server failed to start: %w", err)
		default:
		}
		conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			break
		}
	}

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Toki Editor",
		Width:     1200,
		Height:    800,
		MinWidth:  600,
		MinHeight: 400,
		URL:       fmt.Sprintf("http://%s", addr),
	})

	if err := wailsApp.Run(); err != nil {
		return fmt.Errorf("wails: %w", err)
	}
	return nil
}
