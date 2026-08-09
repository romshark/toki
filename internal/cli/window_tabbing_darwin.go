//go:build darwin && cgo

package cli

/*
#cgo CFLAGS: -x objective-c -Wno-unused-parameter
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void disableAutomaticWindowTabbing() {
	[NSWindow setAllowsAutomaticWindowTabbing:NO];
}
*/
import "C"

// DisableAutomaticWindowTabbing turns off macOS's default behavior of
// merging new windows of the same app into tabs in the active fullscreen
// space. Without this, clicking "new window" while the primary window is
// in native fullscreen creates a browser-style tab instead of a separate
// window. No-op on non-macOS.
func DisableAutomaticWindowTabbing() {
	C.disableAutomaticWindowTabbing()
}
