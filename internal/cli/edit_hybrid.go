package cli

// #cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
import "C"

import (
	"context"
	"fmt"
	"time"

	"github.com/romshark/datapages"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/romshark/toki/editor/app"
)

// windowCascadeOffset is the pixel offset for new windows relative to
// the current window — mirrors the OS-level "cascade" placement.
const windowCascadeOffset = 32

// RunHybridApp launches the editor as a native Wails desktop app backed by
// a local HTTP server on an ephemeral 127.0.0.1 port. Returns when the
// webview window closes or the server fails to start.
//
// This file is compiled only when cgo is enabled (the `import "C"` above
// acts as an implicit build constraint); edit_nocgo.go provides a stub
// used under CGO_ENABLED=0 (e.g. the govulncheck CI job).
func RunHybridApp(a *app.App, s datapages.Server) error {
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	addr, err := startServer(serverCtx, s)
	if err != nil {
		return err
	}

	DisableAutomaticWindowTabbing()

	wailsApp := application.New(application.Options{
		Name: "Toki Editor",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: func() { serverCancel() },
	})

	a.PickDirectory = func(title string) (string, error) {
		return wailsApp.Dialog.OpenFile().
			CanChooseDirectories(true).
			CanChooseFiles(false).
			SetTitle(title).
			PromptForSingleSelection()
	}

	a.OpenNewWindow = func() {
		opts := application.WebviewWindowOptions{
			Title:     "Toki Editor",
			Width:     1200,
			Height:    800,
			MinWidth:  600,
			MinHeight: 400,
			URL:       "http://" + addr,
		}
		if cur := wailsApp.Window.Current(); cur != nil {
			x, y := cur.Position()
			opts.InitialPosition = application.WindowXY
			opts.X = x + windowCascadeOffset
			opts.Y = y + windowCascadeOffset
		}
		wailsApp.Window.NewWithOptions(opts)
	}

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Toki Editor",
		Width:     1200,
		Height:    800,
		MinWidth:  600,
		MinHeight: 400,
		URL:       "http://" + addr,
	})

	if err := wailsApp.Run(); err != nil {
		return fmt.Errorf("wails: %w", err)
	}
	return nil
}

// startServer serves s on a port of 127.0.0.1 that the OS picks and returns
// the address once the port is bound, from when it accepts connections.
// ListenAndServe binds before it serves and reports the bound address through Addr.
func startServer(ctx context.Context, s datapages.Server) (string, error) {
	serverErr := make(chan error, 1)
	go func() { serverErr <- s.ListenAndServe(ctx, "127.0.0.1:0") }()

	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if addr := s.Addr(); addr != "" {
			return addr, nil
		}
		select {
		case err := <-serverErr:
			return "", fmt.Errorf("server failed to start: %w", err)
		case <-tick.C:
		}
	}
}
