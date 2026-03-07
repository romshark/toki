package main

import (
	"os"
	"runtime/debug"
	"time"

	"github.com/romshark/toki/internal/app"
)

// Set by goreleaser via -ldflags -X.
var version, commit, date string

func main() {
	// When built with "go install" the ldflags are not set.
	// Fall back to the build info embedded by the Go toolchain.
	if version == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			version = info.Main.Version
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					commit = s.Value
				case "vcs.time":
					date = s.Value
				}
			}
		}
	}

	app.Version = version
	app.Commit = commit
	app.Date = date
	r, exitCode := app.Run(os.Args, os.Environ(), os.Stderr, os.Stdout, time.Now())
	r.Print()
	os.Exit(exitCode)
}
