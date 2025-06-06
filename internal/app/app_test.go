package app_test

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/romshark/toki/internal/app"
	"github.com/romshark/toki/internal/arb"

	"github.com/romshark/tik/tik-go"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestTokiVersion(t *testing.T) {
	var stderr, stdout bytes.Buffer

	res, exitCode := app.Run([]string{"toki", "version"}, &stderr, &stdout, TimeNow)
	require.Equal(t, 0, exitCode)
	require.Zero(t, res)

	require.Zero(t, stderr.String())
	require.Contains(t, stdout.String(), "Toki v"+app.Version)
}

// TestTokiGenerate tests success for `toki generate` and `toki lint`.
func TestTokiGenerate(t *testing.T) {
	tests := []struct {
		name        string
		setup       Setup
		args        []string
		expectFiles map[string]string // expected files (path -> contents)
	}{
		{
			name: "generate minimal bundle",
			setup: Setup{
				Files: map[string]string{
					"main.go": `
						package main
						import "fmt"
						func main() { fmt.Println("This text isn't localized yet") }
					`,
				},
			},
			args: []string{"-l=en"},
			expectFiles: map[string]string{
				"tokibundle/head.txt": ``,
				"tokibundle/catalog_en.arb": fmt.Sprintf(`{
					"@@locale": "en",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339),
					app.Version,
				),
			},
		},
		{
			name: "omit locale parameter",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
			},
			expectFiles: map[string]string{
				"tokibundle/head.txt": ``,
				"tokibundle/catalog_en.arb": fmt.Sprintf(`{
					"@@locale": "en",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339),
					app.Version,
				),
			},
		},
		{
			name: "provide bundle package path parameter",
			setup: Setup{
				InitGoMod: true,
			},
			args: []string{"-l=en", "-b=pkg/i18n/toki"},
			expectFiles: map[string]string{
				"pkg/i18n/toki/head.txt": ``,
				"pkg/i18n/toki/catalog_en.arb": fmt.Sprintf(`{
					"@@locale": "en",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339),
					app.Version,
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp, resLint, resGenerate := tt.setup.generate(t, TimeNow, tt.args...)
			check := func(t *testing.T, res RunResult) {
				t.Helper()
				require.NoError(t, res.Err)
				require.Equal(t, 0, res.ExitCode)
				if res.Scan.SourceErrors.Len() > 0 {
					for err := range res.Scan.SourceErrors.Seq() {
						t.Errorf("unexpected source error: %v", err)
					}
				}
				require.Zero(t, res.Scan.SourceErrors.Len())
			}
			check(t, resLint)
			check(t, resGenerate)
			checkFiles(t, tmp, tt.expectFiles)
		})
	}
}

type SourceError struct {
	ExpectPosition string
	ExpectErr      require.ErrorAssertionFunc
}

// TestTokiGenerateErr tests errors for `toki generate` and `toki lint`.
func TestTokiGenerateErr(t *testing.T) {
	tests := []struct {
		name           string
		setup          Setup
		args           []string
		expectExitCode int
		expectErr      require.ErrorAssertionFunc
		validate       func(t *testing.T, tmp string)
	}{
		{
			name: "lint empty arb file",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				FilesAfterInit: map[string]string{
					"tokibundle/catalog_en.arb": "{}",
				},
			},
			args:           []string{"-l=en"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, arb.ErrMissingRequiredLocale)
			},
		},
		{
			name: "bundle_gen.go file is broken before init",
			setup: Setup{
				InitGoMod: true,
				FilesAfterInit: map[string]string{
					"main.go":                            "this file is broken",
					"tokibundle/" + app.MainBundleFileGo: "this file is broken",
				},
			},
			args:           []string{"-l=en"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrAnalyzingSource)
			},
		},
		{
			name: "missing locale parameter",
			setup: Setup{
				InitGoMod: true,
			},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrMissingLocaleParam)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEqual(t, 0, tt.expectExitCode,
				"expected exit code must not be 0 OK")

			tmp, resLint, resGenerate := tt.setup.generate(t, TimeNow, tt.args...)
			check := func(t *testing.T, res RunResult) {
				t.Helper()
				tt.expectErr(t, res.Err)
				require.Equal(t, tt.expectExitCode, res.ExitCode)
			}
			check(t, resLint)
			check(t, resGenerate)
			if tt.validate != nil {
				tt.validate(t, tmp)
			}
		})
	}
}

// TestTokiGenerateErrSource tests source errors for
// `toki generate` and `toki lint`.
func TestTokiGenerateErrSource(t *testing.T) {
	errHasMsg := func(expectMsg string) require.ErrorAssertionFunc {
		return func(tt require.TestingT, err error, i ...any) {
			require.EqualError(t, err, expectMsg)
		}
	}

	tests := []struct {
		name          string
		setup         Setup
		args          []string
		expectSrcErrs []SourceError
	}{
		{
			name: "ERR lint argument unexpected",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				FilesAfterInit: map[string]string{
					"main.go": `
					package main
					import "fmt"
					import "time"
					import "tstmod/tokibundle"
					import "golang.org/x/text/language"
					func main() {
						r, _ := tokibundle.Match(language.English)
						fmt.Println(r.String("Expect {\"string\"}", int(42)))
						fmt.Println(r.String("Expect {3}", "a string"))
						fmt.Println(r.String("Expect {10:30 pm}", "a string"))
						fmt.Println(r.String("Broken TIK: {10:40 pm}", time.Now()))
					}
					`,
				},
			},
			args: []string{"lint", "-l=en"},
			expectSrcErrs: []SourceError{
				{"main.go:8:28",
					errHasMsg("TIK: arg 0 must be a string but received: int")},
				{"main.go:9:28",
					errHasMsg("TIK: arg 0 must be numeric but received: string")},
				{"main.go:10:28",
					errHasMsg("TIK: arg 0 must be time.Time but received: string"),
				},
				{"main.go:11:28", func(tt require.TestingT, err error, i ...any) {
					require.ErrorAs(t, err, &tik.ErrParser{})
					require.Equal(t, "TIK: at index 12: unknown placeholder", err.Error())
				}},
			},
		},
		{
			name: "ERR lint extra argument unexpected",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				FilesAfterInit: map[string]string{
					"main.go": `
					package main
					import "fmt"
					import "time"
					import "tstmod/tokibundle"
					import "golang.org/x/text/language"
					func main() {
						r, _ := tokibundle.Match(language.English)
						fmt.Println(r.String(
							"There are no magic constants here",
							int(42),
						))
					}
					`,
				},
			},
			args: []string{"lint", "-l=en"},
			expectSrcErrs: []SourceError{
				{"main.go:9:8",
					errHasMsg("TIK: arg int(42) doesn't match any TIK placeholder")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, resLint, resGenerate := tt.setup.generate(t, TimeNow, tt.args...)
			check := func(t *testing.T, res RunResult) {
				t.Helper()
				require.ErrorIs(t, res.Err, app.ErrSourceErrors)
				require.Equal(t, 1, res.ExitCode)
				index := 0
				for err := range res.Scan.SourceErrors.Seq() {
					if index >= len(tt.expectSrcErrs) {
						t.Errorf("expect source error at index %d", index)
						continue
					}
					expectPosStr := tt.expectSrcErrs[index].ExpectPosition
					pos := err.Position
					base := filepath.Base(pos.String())
					require.Equal(t, expectPosStr, base)
					tt.expectSrcErrs[index].ExpectErr(t, err.Err)
					index++
				}
			}
			check(t, resLint)
			check(t, resGenerate)
		})
	}
}

// BenchmarkOKGenerate benchmarks running `toki generate` after Go module
// and Toki bundle initialization.
func BenchmarkOKGenerate(b *testing.B) {
	s := Setup{InitGoMod: true, InitBundle: true}
	tmp, res, exitCode := s.generate(b, TimeNow, "generate", "-l=en")
	require.Zero(b, exitCode)
	require.NoError(b, res.Err)

	require.NoError(b, os.Chdir(tmp))

	args := []string{"toki", "generate", "-l=en", "-q"}
	for b.Loop() {
		res, exitCode := app.Run(args, os.Stderr, os.Stdout, TimeNow)
		if res.Err != nil {
			b.Fatalf("unexpected error: %v", res.Err)
		}
		if exitCode != 0 {
			b.Fatalf("unexpected exit code: %v", exitCode)
		}
	}
}

func initGoMod(tb testing.TB, dir, name string) {
	tb.Helper()
	cmd := exec.Command("go", "mod", "init", name)
	cmd.Dir = dir
	err := cmd.Run()
	require.NoError(tb, err)
}

var TimeNow = time.Date(2025, 1, 1, 1, 1, 1, 0, time.UTC)

const ModName = "tstmod"

func initBundle(
	tb testing.TB, dir string, locale language.Tag, bundlePkg string,
	stderr, stdout io.Writer,
) app.Result {
	tb.Helper()

	wd, err := os.Getwd()
	require.NoError(tb, err)
	defer func() { _ = os.Chdir(wd) }()
	require.NoError(tb, os.Chdir(dir))

	res, exitCode := app.Run([]string{
		"toki", "generate", "-l", locale.String(), "-b", bundlePkg,
	}, stderr, stdout, TimeNow)
	require.Zero(tb, exitCode)
	return res
}

func writeFiles(tb testing.TB, dir string, files map[string]string) {
	tb.Helper()

	for name, content := range files {
		path := filepath.Join(dir, name)
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(tb, err)
		contents := []byte(strings.TrimSpace(content))
		err = os.WriteFile(path, contents, 0644)
		require.NoError(tb, err)
	}
}

type Setup struct {
	Stderr, Stdout io.Writer
	InitGoMod      bool
	InitBundle     bool
	Files          map[string]string
	FilesAfterInit map[string]string
}

type RunResult struct {
	app.Result
	ExitCode int
}

func (s Setup) generate(
	tb testing.TB, now time.Time, args ...string,
) (dir string, lintResult, generateResult RunResult) {
	tb.Helper()
	dir = tb.TempDir()
	tb.Logf("dir: %s", dir)

	stderr := io.Writer(os.Stderr)
	stdout := io.Writer(os.Stdout)
	if s.Stderr != nil {
		stderr = s.Stderr
	}
	if s.Stdout != nil {
		stdout = s.Stdout
	}

	if s.InitGoMod {
		initGoMod(tb, dir, ModName)
	}
	writeFiles(tb, dir, s.Files)
	if s.InitBundle {
		_ = initBundle(tb, dir, language.English, "tokibundle", stderr, stdout)
	}
	writeFiles(tb, dir, s.FilesAfterInit)

	wd, err := os.Getwd()
	require.NoError(tb, err)
	defer func() { _ = os.Chdir(wd) }()
	require.NoError(tb, os.Chdir(dir))

	ss := snapshotFiles(tb, dir)

	cmd := append([]string{"toki", "lint"}, args...)

	lintResult.Result, lintResult.ExitCode = app.Run(cmd, stderr, stdout, now)

	ss.RequireUnchanged(tb, dir)

	cmd = append([]string{"toki", "generate"}, args...)
	generateResult.Result, generateResult.ExitCode = app.Run(cmd, stderr, stdout, now)
	return dir, lintResult, generateResult
}

func checkFiles(tb testing.TB, dir string, files map[string]string) {
	tb.Helper()
	for path, content := range files {
		full := filepath.Join(dir, path)
		require.FileExists(tb, full)
		c, err := os.ReadFile(full)
		require.NoError(tb, err)
		require.Equal(tb, stripLeadingSpaces(content), stripLeadingSpaces(string(c)))
	}
}

func snapshotFiles(tb testing.TB, root string) FileSnapshot {
	tb.Helper()
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		require.NoError(tb, err)
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		require.NoError(tb, err)
		content, err := os.ReadFile(path)
		require.NoError(tb, err)
		files[rel] = content
		return nil
	})
	require.NoError(tb, err)
	return FileSnapshot{m: files}
}

type FileSnapshot struct{ m map[string][]byte }

func (s FileSnapshot) RequireUnchanged(tb testing.TB, root string) {
	tb.Helper()
	after := snapshotFiles(tb, root)

	// Check all originally present files
	for path, old := range s.m {
		new, ok := after.m[path]
		if !ok {
			tb.Errorf("file unexpectedly deleted: %s", path)
			continue
		}
		if !bytes.Equal(old, new) {
			tb.Errorf("unexpected file change: %s", path)
		}
	}

	// Check for unexpected new files
	for path := range after.m {
		if _, ok := s.m[path]; !ok {
			tb.Errorf("unexpected new file: %s", path)
		}
	}
}

func stripLeadingSpaces(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, " \t")
	}
	return strings.Join(lines, "\n")
}
