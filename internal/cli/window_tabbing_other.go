//go:build !darwin || !cgo

package cli

// DisableAutomaticWindowTabbing is a no-op on non-macOS platforms and in
// non-cgo builds — macOS window tabbing does not apply.
func DisableAutomaticWindowTabbing() {}
