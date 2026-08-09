// Package sqinn embeds a custom sqinn binary built with FTS5 support.
// Use [Path] to get the path to the extracted binary.
package sqinn

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	once     sync.Once
	tempDir  string
	sqinnBin string
	errInit  error
)

// Path returns the filesystem path to the extracted sqinn binary.
// The binary is extracted once from the embedded gzip on first call.
// Subsequent calls return the cached path.
func Path() (string, error) {
	once.Do(func() {
		if len(gzipData) == 0 {
			errInit = os.ErrNotExist
			return
		}
		tempDir, errInit = os.MkdirTemp("", "toki-sqinn-*")
		if errInit != nil {
			return
		}
		exeName := "sqinn"
		if runtime.GOOS == "windows" {
			exeName = "sqinn.exe"
		}
		sqinnBin = filepath.Join(tempDir, exeName)
		gr, err := gzip.NewReader(bytes.NewReader(gzipData))
		if err != nil {
			errInit = err
			return
		}
		f, err := os.OpenFile(sqinnBin, os.O_RDWR|os.O_CREATE, 0o755)
		if err != nil {
			errInit = err
			return
		}
		defer func() { _ = f.Close() }()
		if _, err := io.Copy(f, gr); err != nil {
			errInit = err
		}
	})
	return sqinnBin, errInit
}

// Cleanup removes the temp directory. Call on shutdown.
func Cleanup() {
	if tempDir != "" {
		_ = os.RemoveAll(tempDir)
	}
}
