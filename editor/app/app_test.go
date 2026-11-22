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
	p := newProject(t, "first")
	requireUnsavedEdit := func(a *app.App) {
		t.Helper()
		require.Len(t, a.Changed(), 1)
		require.Equal(t, "de", a.Changed()[0].Catalog.Locale)
		require.Equal(t, "Erste", a.Changed()[0].Message)
	}

	a := p.start()
	require.False(t, a.LoadedFromIndexDB())
	occurrences := findTIK(t, a, "first").Occurrences
	require.Len(t, occurrences, 1)
	require.Equal(t, "main.go", occurrences[0].DisplayFile)
	require.NoError(t, a.SetMessage(findTIK(t, a, "first").ID, "de", "Erste"))

	// Unchanged project.
	a = p.start()
	require.True(t, a.LoadedFromIndexDB())
	require.Equal(t, occurrences, findTIK(t, a, "first").Occurrences)
	requireUnsavedEdit(a)

	// A TIK added to the source without regenerating the bundle.
	p.writeMain("first", "second")
	a = p.start()
	require.False(t, a.LoadedFromIndexDB())
	require.Len(t, a.TIKs(), 2)
	require.Equal(t, 1, a.NumMissing())
	requireUnsavedEdit(a)

	a = p.start()
	require.True(t, a.LoadedFromIndexDB())
	require.Len(t, a.TIKs(), 2)
	require.Equal(t, 1, a.NumMissing())
	require.Len(t, findTIK(t, a, "second").Occurrences, 1)
	requireUnsavedEdit(a)
}

// TestSearchIndexFollowsEdits makes sure search matches the current messages.
func TestSearchIndexFollowsEdits(t *testing.T) {
	p := newProject(t, "first")
	a := p.start()
	id := findTIK(t, a, "first").ID
	search := func(query string) []string {
		t.Helper()
		res, err := p.db.SearchTIKs(query, 0, 10)
		require.NoError(t, err)
		return res.TIKIDs
	}

	require.NoError(t, a.SetMessage(id, "de", "Erste"))
	require.Equal(t, []string{id}, search("Erste"))

	_, err := a.ResetMessage(id, "de")
	require.NoError(t, err)
	require.Empty(t, search("Erste"))
}

// project is a generated Toki project to start the editor on.
type project struct {
	t         *testing.T
	dir       string
	sqinnPath string
	db        *indexdb.DB // Index DB of the last start.
}

// "-mod=mod" lets the scan add the bundle's imports to go.mod.
var env = append(os.Environ(), "GOFLAGS=-mod=mod")

// newProject creates a module whose main.go uses tiks and generates
// its bundle with catalogs for en and de.
func newProject(t *testing.T, tiks ...string) *project {
	t.Helper()
	sqinnPath, err := tokisqinn.Path()
	if err != nil {
		t.Skipf("no sqinn binary for this platform: %v", err)
	}
	p := &project{t: t, dir: t.TempDir(), sqinnPath: sqinnPath}
	t.Cleanup(func() {
		if p.db != nil {
			_ = p.db.Close()
		}
	})

	cmd := exec.Command("go", "mod", "init", "tstmod")
	cmd.Dir = p.dir
	require.NoError(t, cmd.Run())
	p.writeMain(tiks...)
	t.Chdir(p.dir)
	res, exitCode := cli.Run(
		[]string{"toki", "generate", "-l=en", "-t=de"},
		env, io.Discard, io.Discard, time.Now(),
	)
	require.NoError(t, res.Err)
	require.Zero(t, exitCode)
	return p
}

// writeMain writes a main.go that uses tiks.
func (p *project) writeMain(tiks ...string) {
	p.t.Helper()
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
	require.NoError(p.t, os.WriteFile(
		filepath.Join(p.dir, "main.go"), []byte(b.String()), 0o644,
	))
}

// start simulates an editor start, closing the index DB of the previous one.
func (p *project) start() *app.App {
	p.t.Helper()
	if p.db != nil {
		require.NoError(p.t, p.db.Close())
	}
	var err error
	p.db, err = indexdb.Open(filepath.Join(p.dir, ".toki", "index.db"), p.sqinnPath)
	require.NoError(p.t, err)
	a := app.NewApp(p.dir, "tokibundle", env, p.db)
	a.Version = "test" // The index DB is ignored for "dev".
	require.NoError(p.t, a.TryInit())
	return a
}

func findTIK(t *testing.T, a *app.App, raw string) *template.TIK {
	t.Helper()
	for _, tk := range a.TIKs() {
		if tk.TIK == raw {
			return tk
		}
	}
	t.Fatalf("TIK %q not found", raw)
	return nil
}
