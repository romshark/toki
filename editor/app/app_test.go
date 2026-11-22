package app_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/romshark/toki/editor/app"
	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/indexdb"
	tokisqinn "github.com/romshark/toki/editor/sqinn"
	"github.com/romshark/toki/internal/cli"

	"github.com/stretchr/testify/require"
)

// TestTryInitIndexDB makes sure editor starts restore the full state from the
// index DB and rescan the source after it changed.
func TestTryInitIndexDB(t *testing.T) {
	sqinnPath, err := tokisqinn.Path()
	if err != nil {
		t.Skipf("no sqinn binary for this platform: %v", err)
	}
	// "-mod=mod" lets the scan add the bundle's imports to go.mod.
	env := append(os.Environ(), "GOFLAGS=-mod=mod")

	dir := t.TempDir()
	cmd := exec.Command("go", "mod", "init", "tstmod")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
	writeMain := func(tiks ...string) {
		var b strings.Builder
		b.WriteString(`package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
		`)
		for _, s := range tiks {
			fmt.Fprintf(&b, "fmt.Println(r.String(%q))\n", s)
		}
		b.WriteString("}\n")
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "main.go"), []byte(b.String()), 0o644,
		))
	}
	writeMain("first")
	t.Chdir(dir)
	res, exitCode := cli.Run(
		[]string{"toki", "generate", "-l=en", "-t=de"},
		env, io.Discard, io.Discard, time.Now(),
	)
	require.NoError(t, res.Err)
	require.Zero(t, exitCode)

	// start simulates an editor start on dir.
	var db *indexdb.DB
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})
	start := func() *app.App {
		t.Helper()
		if db != nil {
			require.NoError(t, db.Close())
		}
		db, err = indexdb.Open(filepath.Join(dir, ".toki", "index.db"), sqinnPath)
		require.NoError(t, err)
		a := app.NewApp(dir, "tokibundle", env, db)
		a.Version = "test" // The index DB is ignored for "dev".
		require.NoError(t, a.TryInit())
		return a
	}
	tik := func(a *app.App, raw string) *template.TIK {
		t.Helper()
		for _, tk := range a.TIKs() {
			if tk.TIK == raw {
				return tk
			}
		}
		t.Fatalf("TIK %q not found", raw)
		return nil
	}
	requireUnsavedEdit := func(a *app.App) {
		t.Helper()
		require.Len(t, a.Changed(), 1)
		require.Equal(t, "de", a.Changed()[0].Catalog.Locale)
		require.Equal(t, "Erste", a.Changed()[0].Message)
	}

	a := start()
	require.False(t, a.LoadedFromIndexDB())
	occurrences := tik(a, "first").Occurrences
	require.Len(t, occurrences, 1)
	require.Equal(t, "main.go", occurrences[0].DisplayFile)
	require.NoError(t, a.SetMessage(tik(a, "first").ID, "de", "Erste"))

	// Unchanged project.
	a = start()
	require.True(t, a.LoadedFromIndexDB())
	require.Equal(t, occurrences, tik(a, "first").Occurrences)
	requireUnsavedEdit(a)

	// A TIK added to the source without regenerating the bundle.
	writeMain("first", "second")
	a = start()
	require.False(t, a.LoadedFromIndexDB())
	require.Len(t, a.TIKs(), 2)
	require.Equal(t, 1, a.NumMissing())
	requireUnsavedEdit(a)

	a = start()
	require.True(t, a.LoadedFromIndexDB())
	require.Len(t, a.TIKs(), 2)
	require.Equal(t, 1, a.NumMissing())
	require.Len(t, tik(a, "second").Occurrences, 1)
	requireUnsavedEdit(a)
}
