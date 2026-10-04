package main

import (
	"os"
	"runtime/debug"
	"time"

	"github.com/romshark/toki/internal/cli"
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

	cli.Version = version
	cli.Commit = commit
	cli.Date = date
	r, exitCode := cli.Run(os.Args, os.Environ(), os.Stderr, os.Stdout, time.Now())
	r.Print()
	os.Exit(exitCode)
}
