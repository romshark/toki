package codeparse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/romshark/toki/internal/codeparse"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/tik/tik-go"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

// TestCollectARBFilesClosesFiles makes sure repeated scans,
// like the editor's builds, don't leak open catalog files.
func TestCollectARBFilesClosesFiles(t *testing.T) {
	openFiles := func() int {
		t.Helper()
		entries, err := os.ReadDir("/dev/fd")
		if err != nil {
			t.Skipf("can't count open files: %v", err)
		}
		return len(entries)
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog_en.arb"),
		[]byte(`{"@@locale": "en", "msg": "Hello"}`), 0o644))
	p := codeparse.NewParser(xxhash.New(),
		tik.NewParser(tik.DefaultConfig), tik.NewICUTranslator(tik.DefaultConfig))

	before := openFiles()
	for range 10 {
		scan := codeparse.NewScan(language.English, "")
		require.NoError(t, p.CollectARBFiles(dir, scan))
		require.Equal(t, 1, scan.Catalogs.Len())
	}
	require.Equal(t, before, openFiles())
}
