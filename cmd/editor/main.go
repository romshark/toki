package main

// #cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
import "C"

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/romshark/toki/editor"
	intapp "github.com/romshark/toki/internal/app"
	"github.com/romshark/toki/internal/log"
)

func main() {
	bundlePkg := flag.String("b", "tokibundle", "path to generated Go bundle package")
	flag.Parse()

	dir, _ := filepath.Abs(".")
	log.SetWriter(os.Stderr, false)

	a, s := editor.Setup(dir, *bundlePkg, os.Environ(), intapp.CleanGenerated, intapp.GenerateBundle)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
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
		OnShutdown: func() {
			serverCancel()
		},
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

	for {
		select {
		case err := <-serverErr:
			fmt.Fprintf(os.Stderr, "server failed to start: %v\n", err)
			os.Exit(1)
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
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
		fmt.Fprintf(os.Stderr, "wails: %v\n", err)
		os.Exit(1)
	}
}
