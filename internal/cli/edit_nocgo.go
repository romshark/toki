//go:build !cgo

package cli

import (
	"errors"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app"
)

// RunHybridApp is a stub used when cgo is disabled (e.g. the govulncheck
// CI job under CGO_ENABLED=0). The real Wails-backed implementation lives
// in edit_hybrid.go and is only compiled with cgo.
func RunHybridApp(_ *app.App, _ datapages.Server) error {
	return errors.New(
		"this build of toki was compiled without cgo; " +
			"the native desktop app is unavailable — " +
			"run `toki edit -server <addr>` instead",
	)
}
