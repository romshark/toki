package main

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

func TestVersion(t *testing.T) {
	var stderr, stdout bytes.Buffer

	res, exitCode := app.Run(
		[]string{"toki", "version"}, osEnv(), &stderr, &stdout, TimeNow,
	)
	require.Equal(t, 0, exitCode)
	require.Zero(t, res)

	require.Zero(t, stderr.String())
	require.Contains(t, stdout.String(), "Toki v"+app.Version)
}

func TestGenerateAndRun(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import (
				"os"
				"fmt"
				"tstmod/tokibundle"
				"golang.org/x/text/language"
			)
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("just text"))
				fmt.Println(r.String("It's okay!"))
				fmt.Println(r.String("with {text}", "something"))
				_, _ = r.Write(os.Stdout, "write to stdout writer")
			}
		`,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate", "-l=en"}
		result, exitCode := app.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	expect := stripLeadingSpaces(strings.TrimSpace(`
		just text
		It's okay!
		with something
		write to stdout writer
	`))
	actual := stripLeadingSpaces(strings.TrimSpace(string(out)))
	require.Equal(t, expect, actual)
}

// TestGenerateAndRunFallback tests fallback to available catalogs when no catalog
// matches a particular locale.
func TestGenerateAndRunFallback(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				rEN, _ := tokibundle.Match(language.English)
				rDE, _ := tokibundle.Match(language.German)
				rES, _ := tokibundle.Match(language.Spanish) // Fall back to German.
				rZU, _ := tokibundle.Match(language.Zulu) // Fall back to English.
				fmt.Println("EN:", rEN.String("translated")) // translated
				fmt.Println("DE:", rDE.String("translated")) // übersetzt
				fmt.Println("ES:", rES.String("translated")) // fallback to DE
				fmt.Println("ZU:", rZU.String("translated")) // fallback to EN
			}
		`,
		"tokibundle/catalog_de.arb": `{
			"@@locale": "de",
			"@@last_modified": "2025-01-01T01:01:01Z",
			"@@x-generator": "github.com/romshark/toki",
			"@@x-generator-version": "0.7.1",
			"msg1bf544c92a992298": "übersetzt",
			"@msg1bf544c92a992298": {
					"description": "übersetzt",
					"type": "text"
			}
		}`,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate", "-l=en", "-t=de"}
		result, exitCode := app.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate"}
		result, exitCode := app.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	expect := stripLeadingSpaces(strings.TrimSpace(`
		EN: translated
		DE: übersetzt
		ES: übersetzt
		ZU: translated
	`))
	actual := stripLeadingSpaces(strings.TrimSpace(string(out)))
	require.Equal(t, expect, actual)
}

// TestGenerateAndRunNoTranslationFallback tests falling back to default language
// when no translation is avaiable for a particular TIK.
func TestGenerateAndRunNoTranslationFallback(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				rEN, _ := tokibundle.Match(language.English)
				rDE, _ := tokibundle.Match(language.German)
				rES, _ := tokibundle.Match(language.Spanish) // Fall back to German.
				rZU, _ := tokibundle.Match(language.Zulu) // Fall back to English.
				fmt.Println("EN:", rEN.String("translated")) // translated
				fmt.Println("DE:", rDE.String("translated")) // übersetzt
				fmt.Println("ES:", rES.String("translated")) // fallback to DE
				fmt.Println("ZU:", rZU.String("translated")) // fallback to EN
			}
		`,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate", "-l=en", "-t=de"}
		result, exitCode := app.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate"}
		result, exitCode := app.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	expect := stripLeadingSpaces(strings.TrimSpace(`
		EN: translated
		DE: translated
		ES: translated
		ZU: translated
	`))
	actual := stripLeadingSpaces(strings.TrimSpace(string(out)))
	require.Equal(t, expect, actual)
}

// TestGenerate tests success for `toki generate` and `toki lint`.
func TestGenerate(t *testing.T) {
	tests := []struct {
		name        string
		setup       Setup
		args        []string
		expectFiles map[string]string // expected files (path -> contents)
	}{
		{
			name: "generate minimal bundle",
			setup: Setup{
				InitGoMod: true,
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
					TimeNow.Format(time.RFC3339), app.Version),
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
					TimeNow.Format(time.RFC3339), app.Version),
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
					TimeNow.Format(time.RFC3339), app.Version),
			},
		},
		{
			name: "provide multiple translation parameters",
			setup: Setup{
				InitGoMod: true,
			},
			args: []string{
				"-l=en", "-b=pkg/i18n/toki",
				"-t=de", "-t=en-US",
				"-t=de", // Intentional duplicate.
			},
			// TODO: make sure the .go files exist too.
			expectFiles: map[string]string{
				"pkg/i18n/toki/head.txt": ``,
				"pkg/i18n/toki/catalog_en.arb": fmt.Sprintf(`{
					"@@locale": "en",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339), app.Version),
				"pkg/i18n/toki/catalog_de.arb": fmt.Sprintf(`{
					"@@locale": "de",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339), app.Version),
				"pkg/i18n/toki/catalog_en_us.arb": fmt.Sprintf(`{
					"@@locale": "en-US",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339), app.Version),
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
					for err := range res.Scan.SourceErrors.SeqRead() {
						t.Errorf("unexpected source error: %v", err)
					}
				}
				require.Zero(t, res.Scan.SourceErrors.Len())
			}
			check(t, resLint)
			check(t, resGenerate)
			checkFiles(t, tmp, tt.expectFiles)
			// TODO: make sure the program is compilable.
		})
	}
}

type SourceError struct {
	ExpectPosition string
	ExpectErr      require.ErrorAssertionFunc
}

// TestGenerateErr tests errors for `toki generate` and `toki lint`.
func TestGenerateErr(t *testing.T) {
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
				require.Equal(t, "analyzing sources: searching .arb files: "+
					"parsing .arb file: missing required @@locale", err.Error())
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
				require.ErrorContains(t, err, "analyzing sources: errors in package")
				require.ErrorContains(t, err, "expected 'package'")
			},
		},
		{
			name: "main.go file broken",
			setup: Setup{
				InitGoMod: true,
				Files: map[string]string{
					// A closing " is missing before `, "something"`.
					"main.go": `
						package main
						import (
							"fmt"
							"example/i18n/tokibundle"
						)
						func main() {
							reader, _ := tokibundle.Default()
							fmt.Println(reader.String("with {text}, "something"))
						}
					`,
				},
			},
			args:           []string{"-l=en"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrAnalyzingSource)
				require.ErrorContains(t, err,
					`analyzing sources: errors in package "main"`)
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
				require.Equal(t, "please provide a valid non-und BCP 47 locale for the "+
					"default language of your original code base "+
					"using the 'l' parameter", err.Error())
			},
		},
		{
			name: "invalid default locale parameter",
			setup: Setup{
				InitGoMod: true,
			},
			expectExitCode: 2,
			args:           []string{"-l=invalid"},
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrInvalidCLIArgs)
				require.Equal(t, `invalid arguments: argument l="invalid": `+
					"must be a valid non-und BCP 47 locale: "+
					"language: tag is not well-formed", err.Error())
			},
		},
		{
			name: "default locale parameter is und",
			setup: Setup{
				InitGoMod: true,
			},
			expectExitCode: 2,
			args:           []string{"-l=und"},
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrInvalidCLIArgs)
				require.Equal(t, `invalid arguments: argument l="und": `+
					"must be a valid non-und BCP 47 locale: is und", err.Error())
			},
		},
		{
			name: "invalid translation locale parameter",
			setup: Setup{
				InitGoMod: true,
			},
			expectExitCode: 2,
			args:           []string{"-l=en", "-t=invalid"},
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrInvalidCLIArgs)
				require.Equal(t, `invalid arguments: argument t="invalid": `+
					"must be a valid non-und BCP 47 locale: "+
					"language: tag is not well-formed", err.Error())
			},
		},
		{
			name: "translation locale parameter is und",
			setup: Setup{
				InitGoMod: true,
			},
			expectExitCode: 2,
			args:           []string{"-l=en", "-t=und"},
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrInvalidCLIArgs)
				require.Equal(t, `invalid arguments: argument t="und": `+
					"must be a valid non-und BCP 47 locale: is und", err.Error())
			},
		},
		{
			name: "catalog has wrong locale metafield",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				FilesAfterInit: map[string]string{
					"main.go": `
						package main
						import "fmt"
						import "tstmod/tokibundle"
						import "golang.org/x/text/language"
						func main() {
							r, _ := tokibundle.Match(language.English)
							fmt.Println(r.String("localized string"))
						}
					`,
					// The @@locale field must be "de", not "en".
					"tokibundle/catalog_de.arb": fmt.Sprintf(`
						{
							"@@locale": "en",
							"@@last_modified": "2025-06-06T01:29:56+02:00",
							"@@x-generator": "github.com/romshark/toki",
							"@@x-generator-version": %q,
							"msg1bf544c92a992298": "übersetzt",
							"@msg1bf544c92a992298": {
								"type": "text"
							}
						}
					`, app.Version),
				},
			},
			args:           []string{"-l=en"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrAnalyzingSource)
				require.Equal(t, "analyzing sources: searching .arb files: "+
					"locale in ARB file (en) differs from file name (de): "+
					"catalog_de.arb", err.Error())
			},
		},
		{
			name: "require completeness",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				Files: map[string]string{
					"main.go": `
						package main
						import "tstmod/tokibundle"
						func main() {
							r := tokibundle.Default()
							print(r.String("There are {# errors}", 0))
						}
					`,
				},
			},
			args:           []string{"-require-complete"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, app.ErrBundleIncomplete)
				require.Equal(t, "bundle contains incomplete catalogs", err.Error())
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

// TestGenerateErrSource tests source errors for
// `toki generate` and `toki lint`.
func TestGenerateErrSource(t *testing.T) {
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
						fmt.Println(r.String("Expect {text}", int(42)))
						fmt.Println(r.String("Expect {integer}", "a string"))
						fmt.Println(r.String("Expect {integer}", 2.5))
						fmt.Println(r.String("Expect {number}", int(42)))
						fmt.Println(r.String("Expect {time-short}", "2025-06-08T10:02:06+00:00"))
						fmt.Println(r.String("Broken TIK: {time-verylong}", time.Now()))
					}
					`,
				},
			},
			args: []string{"lint", "-l=en"},
			expectSrcErrs: []SourceError{
				{
					"main.go:8:28",
					errHasMsg("TIK: arg 0 must be a string but received: int"),
				},
				{
					"main.go:9:28",
					errHasMsg("TIK: arg 0 must be an integer but received: string"),
				},
				{
					"main.go:10:28",
					errHasMsg("TIK: arg 0 must be an integer but received: float64"),
				},
				{
					"main.go:11:28",
					errHasMsg("TIK: arg 0 must be a float but received: int"),
				},
				{
					"main.go:12:28",
					errHasMsg("TIK: arg 0 must be time.Time but received: string"),
				},
				{"main.go:13:28", func(tt require.TestingT, err error, i ...any) {
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
					import "tstmod/tokibundle"
					func main() {
						fmt.Println(tokibundle.Default().String(
							"There are no placeholders here",
							int(42),
						))
					}
					`,
				},
			},
			args: []string{"lint", "-l=en"},
			expectSrcErrs: []SourceError{
				{
					"main.go:6:8",
					errHasMsg("TIK: arg int(42) doesn't match any TIK placeholder"),
				},
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
				for err := range res.Scan.SourceErrors.SeqRead() {
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
		res, exitCode := app.Run(args, osEnv(), os.Stderr, os.Stdout, TimeNow)
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

func runInDir(t testing.TB, dir string, fn func()) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(wd) }()
	require.NoError(t, os.Chdir(dir))
	fn()
}

func initBundle(
	tb testing.TB, dir string, locale language.Tag, bundlePkg string,
	stderr, stdout io.Writer,
) (result app.Result) {
	tb.Helper()

	runInDir(tb, dir, func() {
		var exitCode int
		result, exitCode = app.Run([]string{
			"toki", "generate", "-l", locale.String(), "-b", bundlePkg,
		}, osEnv(), stderr, stdout, TimeNow)
		require.Zero(tb, exitCode)
	})
	return result
}

func writeFiles(tb testing.TB, dir string, files map[string]string) {
	tb.Helper()

	for name, content := range files {
		path := filepath.Join(dir, name)
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(tb, err)
		contents := []byte(strings.TrimSpace(content))
		err = os.WriteFile(path, contents, 0o644)
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

	runInDir(tb, dir, func() {
		a := append([]string{"toki", "generate"}, args...)
		generateResult.Result, generateResult.ExitCode = app.Run(
			a, osEnv(), stderr, stdout, now,
		)

		ss := snapshotFiles(tb, dir)
		a = append([]string{"toki", "lint"}, args...)
		lintResult.Result, lintResult.ExitCode = app.Run(
			a, osEnv(), stderr, stdout, now,
		)
		ss.RequireUnchanged(tb, dir)
	})
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

func osEnv() []string {
	// go test sets GOFLAGS to “-mod=readonly”, which prevents any nested go commands
	// (those run by packages.Load, `go list`, or `go run` inside this test) from
	// updating go.mod when new imports appear in the files generated by Toki.
	// Overriding it with “-mod=mod” lets those inner commands record the missing
	// dependencies automatically, so the second analysis pass and the final `go run`
	// succeed.
	return append(os.Environ(), "GOFLAGS=-mod=mod")
}
