package app_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/romshark/datapages"
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

// TestSetLocaleOrder makes sure the order of the locales that the user sets
// orders the catalogs and the messages of every TIK, survives a restart and
// must list each locale once.
func TestSetLocaleOrder(t *testing.T) {
	p := newProject(t, "first", "second")
	requireOrder := func(a *app.App, want ...string) {
		t.Helper()
		var catalogs []string
		for _, c := range a.Catalogs() {
			catalogs = append(catalogs, c.Locale)
		}
		require.Equal(t, want, catalogs)
		for _, tk := range a.TIKs() {
			var messages []string
			for _, m := range tk.ICU {
				messages = append(messages, m.Catalog.Locale)
			}
			require.Equal(t, want, messages, tk.TIK)
		}
	}

	a := p.start()
	requireOrder(a, "en", "de") // The default locale first.

	changed, err := a.SetLocaleOrder([]string{"de", "en"})
	require.NoError(t, err)
	require.True(t, changed)
	requireOrder(a, "de", "en")

	changed, err = a.SetLocaleOrder([]string{"de", "en"})
	require.NoError(t, err)
	require.False(t, changed)

	for _, order := range [][]string{
		{"de"}, {"de", "de"}, {"de", "fr"}, {"de", "en", "fr"},
	} {
		_, err := a.SetLocaleOrder(order)
		require.ErrorIs(t, err, datapages.ErrBadRequest, order)
	}
	requireOrder(a, "de", "en")

	a = p.start()
	require.True(t, a.LoadedFromIndexDB())
	requireOrder(a, "de", "en")
}

// TestSetMessageReadOnly makes sure the server rejects edits of messages
// the TIK fully determines.
func TestSetMessageReadOnly(t *testing.T) {
	p := newProject(t, "first")
	a := p.start()
	tk := findTIK(t, a, "first")
	i := slices.IndexFunc(tk.ICU, func(m *template.ICUMessage) bool {
		return m.Catalog.Locale == "en"
	})
	require.True(t, tk.ICU[i].IsReadOnly, "native message of a plain TIK")

	err := a.SetMessage(tk.ID, "en", "changed")
	require.ErrorIs(t, err, datapages.ErrForbidden)
	require.Equal(t, "first", tk.ICU[i].Message)
	require.Empty(t, a.Changed())
}

// TestSearchIndexFollowsEdits makes sure search matches the current messages.
func TestSearchIndexFollowsEdits(t *testing.T) {
	p := newProject(t, "first")
	a := p.start()
	id := findTIK(t, a, "first").ID
	search := func(query string) []string {
		t.Helper()
		ids, err := p.db.SearchTIKs(query)
		require.NoError(t, err)
		return ids
	}

	require.NoError(t, a.SetMessage(id, "de", "Erste"))
	require.Equal(t, []string{id}, search("Erste"))

	_, err := a.ResetMessage(id, "de")
	require.NoError(t, err)
	require.Empty(t, search("Erste"))
}

// TestSearchFiltersAndPages makes sure search results go through the domain
// and status filters and the page index is clamped before the page is built.
func TestSearchFiltersAndPages(t *testing.T) {
	p := newProject(t, "apple one", "apple two", "banana")
	a := p.start()
	require.NoError(t, a.SetMessage(findTIK(t, a, "apple one").ID, "de", "Apfel eins"))
	search := func(
		filterType string, showDomains map[string]bool, pageIdx int,
	) (pageIdxShown int, tiks []string) {
		t.Helper()
		data := a.BuildFilteredDataIndex(
			filterType, nil, showDomains, pageIdx, 10, "apple",
		)
		for _, tk := range data.TIKs {
			tiks = append(tiks, tk.TIK)
		}
		return data.PageIdx, tiks
	}

	_, tiks := search("changed", nil, 0)
	require.Equal(t, []string{"apple one"}, tiks)

	_, tiks = search("", map[string]bool{"other": true}, 0)
	require.Empty(t, tiks)

	pageIdx, tiks := search("", nil, 5)
	require.Zero(t, pageIdx)
	require.ElementsMatch(t, []string{"apple one", "apple two"}, tiks)
}

// TestDomainSignalKey makes sure domain signal keys are identifiers
// that decode back to the domain names they encode.
func TestDomainSignalKey(t *testing.T) {
	shown := map[string]bool{
		"app.web":   true,
		"app_web":   false,
		"my-domain": true,
		"ünï cödé":  false,
	}
	keys := make(map[string]bool, len(shown))
	for name, v := range shown {
		keys[app.DomainSignalKey(name)] = v
	}
	require.Equal(t, shown, app.NormalizeDomainsSignal(keys))
	for key := range keys {
		require.Regexp(t, `^[A-Za-z_][A-Za-z0-9_]*$`, key)
	}
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
