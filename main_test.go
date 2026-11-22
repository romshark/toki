package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	editorapp "github.com/romshark/toki/editor/app"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/cli"
	"github.com/romshark/toki/internal/codeparse"

	"github.com/romshark/tik/tik-go"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestVersion(t *testing.T) {
	var stderr, stdout bytes.Buffer

	res, exitCode := cli.Run(
		[]string{"toki", "version"}, osEnv(), &stderr, &stdout, TimeNow,
	)
	require.Equal(t, 0, exitCode)
	require.Zero(t, res)

	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "Toki "+cli.Version)
	require.Contains(t, stdout.String(), runtime.Version())
	require.NotContains(t, stdout.String(), "\ndep\t")
}

func TestVersionFull(t *testing.T) {
	var stderr, stdout bytes.Buffer

	res, exitCode := cli.Run(
		[]string{"toki", "version", "-full"}, osEnv(), &stderr, &stdout, TimeNow,
	)
	require.Equal(t, 0, exitCode)
	require.Zero(t, res)

	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "Toki "+cli.Version)
	require.Contains(t, stdout.String(), "\ndep\tgithub.com/stretchr/testify")
}

func TestVersionFlag(t *testing.T) {
	for _, arg := range []string{"-version", "--version"} {
		t.Run(arg, func(t *testing.T) {
			var stderr, stdout bytes.Buffer
			res, exitCode := cli.Run(
				[]string{"toki", arg}, osEnv(), &stderr, &stdout, TimeNow,
			)
			require.Equal(t, 0, exitCode)
			require.Zero(t, res)
			require.Contains(t, stdout.String(), "Toki "+cli.Version)
		})
	}
}

func TestHelp(t *testing.T) {
	commands := []string{"generate", "lint", "edit", "repair", "version", "help"}

	t.Run("no command", func(t *testing.T) {
		var stderr, stdout bytes.Buffer
		res, exitCode := cli.Run([]string{"toki"}, osEnv(), &stderr, &stdout, TimeNow)
		require.Equal(t, 2, exitCode)
		require.Zero(t, res)
		require.Empty(t, stdout.String())
		require.Contains(t, stderr.String(), "Usage:")
	})

	for _, args := range [][]string{
		{"help"}, {"-h"}, {"-help"}, {"--help"}, {"help", "help"}, {"help", "-h"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stderr, stdout bytes.Buffer
			res, exitCode := cli.Run(
				append([]string{"toki"}, args...), osEnv(), &stderr, &stdout, TimeNow,
			)
			require.Equal(t, 0, exitCode)
			require.Zero(t, res)
			require.Empty(t, stderr.String())
			require.Contains(t, stdout.String(), "Usage:")
			for _, c := range commands {
				require.Contains(t, stdout.String(), "\t"+c+" ")
			}
		})
	}

	for _, c := range commands[:len(commands)-1] {
		for _, args := range [][]string{
			{"help", c}, {c, "-h"}, {c, "-help"}, {c, "--help"},
		} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				var stderr, stdout bytes.Buffer
				res, exitCode := cli.Run(
					append([]string{"toki"}, args...), osEnv(), &stderr, &stdout, TimeNow,
				)
				require.Equal(t, 0, exitCode)
				require.Zero(t, res)
				require.Empty(t, stderr.String())
				require.True(t, strings.HasPrefix(stdout.String(), "Usage: toki "+c),
					"unexpected usage: %q", stdout.String())
			})
		}
	}
}

func TestCLIArgsErr(t *testing.T) {
	tests := []struct {
		args      []string
		expectErr error
	}{
		{[]string{"unknown"}, cli.ErrUnknownCommand},
		{[]string{"help", "unknown"}, cli.ErrUnknownCommand},
		{[]string{"generate", "-unknown"}, cli.ErrInvalidCLIArgs},
		{[]string{"lint", "-unknown"}, cli.ErrInvalidCLIArgs},
		{[]string{"edit", "-unknown"}, cli.ErrInvalidCLIArgs},
		{[]string{"repair", "-unknown"}, cli.ErrInvalidCLIArgs},
		{[]string{"version", "-unknown"}, cli.ErrInvalidCLIArgs},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stderr, stdout bytes.Buffer
			res, exitCode := cli.Run(
				append([]string{"toki"}, tt.args...), osEnv(), &stderr, &stdout, TimeNow,
			)
			require.Equal(t, 2, exitCode)
			require.ErrorIs(t, res.Err, tt.expectErr)
			require.Empty(t, stderr.String())
			require.Empty(t, stdout.String())
		})
	}
}

func TestGenerateAndRun(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import (
				"fmt"
				"os"
				"time"
				"tstmod/tokibundle"

				"github.com/go-playground/locales/currency"
				"golang.org/x/text/language"
			)
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("just text"))
				fmt.Println(r.String("It's okay!")) // Escaping single quote.
				_, _ = r.Write(os.Stdout, "write to stdout writer")

				fmt.Println(r.String("text: {text}", "something"))
				fmt.Println(r.String(
					"texts: {text}|{text}|{text}|{text}|{text}|{text}|{text}|{text}|{text}|{text}|{text}",
					"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
				))

				fmt.Println(r.String("number: {number}", 3.1415912))
				fmt.Println(r.String("number: {number}", float64(10.1000)))
				fmt.Println(r.String("number: {number}", float32(123)))

				fmt.Println(r.String("integer: {integer}", 1024))
				fmt.Println(r.String("integer: {integer}", byte(42)))
				fmt.Println(r.String("integer: {integer}", int8(42)))
				fmt.Println(r.String("integer: {integer}", uint8(42)))
				fmt.Println(r.String("integer: {integer}", int16(42)))
				fmt.Println(r.String("integer: {integer}", uint16(42)))
				fmt.Println(r.String("integer: {integer}", int32(42)))
				fmt.Println(r.String("integer: {integer}", uint32(42)))
				fmt.Println(r.String("integer: {integer}", int64(42)))
				fmt.Println(r.String("integer: {integer}", uint64(42)))
				fmt.Println(r.String("integer: {integer}", int(42)))
				fmt.Println(r.String("integer: {integer}", uint(42)))

				tm := time.Date(2025, 7, 14, 10, 11, 12, 0, time.UTC)
				fmt.Println(r.String("date-full: {date-full}", tm))
				fmt.Println(r.String("date-long: {date-long}", tm))
				fmt.Println(r.String("date-medium: {date-medium}", tm))
				fmt.Println(r.String("date-short: {date-short}", tm))
				fmt.Println(r.String("time-full: {time-full}", tm))
				fmt.Println(r.String("time-long: {time-long}", tm))
				fmt.Println(r.String("time-medium: {time-medium}", tm))
				fmt.Println(r.String("time-short: {time-short}", tm))

				fmt.Println(r.String("currency: {currency}",
					tokibundle.Currency{Amount: 3.99, Type: currency.USD}))
			}
		`,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate", "-l=en"}
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
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
		write to stdout writertext: something
		texts: 0|1|2|3|4|5|6|7|8|9|10
		number: 3.1415912
		number: 10.1
		number: 123
		integer: 1024
		integer: 42
		integer: 42
		integer: 42
		integer: 42
		integer: 42
		integer: 42
		integer: 42
		integer: 42
		integer: 42
		integer: 42
		integer: 42
		date-full: Monday, July 14, 2025
		date-long: July 14, 2025
		date-medium: Jul 14, 2025
		date-short: 7/14/25
		time-full: 10:11:12 am UTC
		time-long: 10:11:12 am UTC
		time-medium: 10:11:12 am
		time-short: 10:11 am
		currency: $3.99
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
			"@@x-generator-version": "0.8.2",
			"msg9d21e6c2519b8be9": "übersetzt",
			"@msg9d21e6c2519b8be9": {
					"description": "übersetzt",
					"type": "text"
			}
		}`,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate", "-l=en", "-t=de"}
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate"}
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
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
// when no translation is available for a particular TIK.
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
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate"}
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
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

// TestGenerateAndRunBacktick makes sure TIKs containing backticks
// generate valid Go.
func TestGenerateAndRunBacktick(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("Run \x60toki generate\x60 first"))
			}
		`,
	})
	_ = initBundle(t, dir, language.English, "tokibundle", io.Discard, io.Discard)

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	require.Equal(t, "Run `toki generate` first", strings.TrimSpace(string(out)))
}

// TestGenerateMultiLineTIK makes sure multi-line TIKs
// stay readable raw strings in the generated bundle.
func TestGenerateMultiLineTIK(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("first line\nsecond line"))
			}
		`,
	})
	_ = initBundle(t, dir, language.English, "tokibundle", io.Discard, io.Discard)

	bundle, err := os.ReadFile(filepath.Join(dir, "tokibundle", "bundle_gen.go"))
	require.NoError(t, err)
	require.Contains(t, string(bundle), "= `first line\nsecond line`\n")
}

// TestGenerateAndRunSelectOrdinal makes sure selectordinal messages
// pick their option by the locale's ordinal plural rules.
func TestGenerateAndRunSelectOrdinal(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				for _, n := range []int{1, 2, 3, 4, 11, 12, 13, 21, 22, 23} {
					fmt.Println(r.String("You finished {ordinal}!", n))
				}
			}
		`,
	})

	_ = initBundle(t, dir, language.English, "tokibundle", io.Discard, io.Discard)

	// Add the English ordinal options the TIK can't produce.
	setOnlyMessage(t, filepath.Join(dir, "tokibundle", "catalog_en.arb"),
		"{var0, selectordinal, "+
			"one {You finished #st!} two {You finished #nd!} "+
			"few {You finished #rd!} other {You finished #th!}}")
	_ = initBundle(t, dir, language.English, "tokibundle", io.Discard, io.Discard)

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	expect := stripLeadingSpaces(strings.TrimSpace(`
		You finished 1st!
		You finished 2nd!
		You finished 3rd!
		You finished 4th!
		You finished 11th!
		You finished 12th!
		You finished 13th!
		You finished 21st!
		You finished 22nd!
		You finished 23rd!
	`))
	actual := stripLeadingSpaces(strings.TrimSpace(string(out)))
	require.Equal(t, expect, actual)
}

// TestGenerateAndRunApostrophes makes sure apostrophes in catalogs
// edited by translators are unescaped like ICU does.
func TestGenerateAndRunApostrophes(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, ModName)
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.German)
				fmt.Println(r.String("apostrophe"))
				fmt.Println(r.String("doubled apostrophe"))
				fmt.Println(r.String("quoted braces"))
				fmt.Println(r.String("number sign"))
				fmt.Println(r.String("plural {# files}", 2))
				fmt.Println(r.String("select {text}", "a"))
			}
		`,
	})

	generate := func() {
		runInDir(t, dir, func() {
			args := []string{"toki", "generate", "-l=en"}
			result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
			require.NoError(t, result.Err)
			require.Zero(t, exitCode)
		})
	}
	generate()

	// Native messages the TIK fully determines are locked.
	// Translate them in a German catalog derived from the native one.
	catalog := filepath.Join(dir, "tokibundle", "catalog_en.arb")
	contents, err := os.ReadFile(catalog)
	require.NoError(t, err)
	patched := strings.Replace(string(contents),
		`"@@locale": "en"`, `"@@locale": "de"`, 1)
	for _, r := range [...]struct{ old, new string }{
		{"apostrophe", "He's here"},
		{"doubled apostrophe", "It''s fine"},
		{"quoted braces", "'{'braces'}'"},
		{"number sign", "Use '#' here"},
		{
			"plural {var0, plural, other {# files}}",
			"{var0, plural, other {# files, '#'1, it's ok}}",
		},
		{"select {var0}", "{var0, select, a {He''s} other {They''re}}"},
	} {
		before := patched
		patched = strings.Replace(patched, `"`+r.old+`"`, `"`+r.new+`"`, 1)
		require.NotEqual(t, before, patched, "%q not found in %s", r.old, catalog)
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "tokibundle", "catalog_de.arb"), []byte(patched), 0o644,
	))
	generate()

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	expect := stripLeadingSpaces(strings.TrimSpace(`
		He's here
		It's fine
		{braces}
		Use '#' here
		2 files, #1, it's ok
		He's
	`))
	actual := stripLeadingSpaces(strings.TrimSpace(string(out)))
	require.Equal(t, expect, actual)
}

// TestGenerateAndRunPluralExactAndOffset makes sure plurals match =n options
// against the argument and pick the plural rule for the argument minus the offset.
func TestGenerateAndRunPluralExactAndOffset(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				for _, n := range []int{0, 1, 2, 3} {
					fmt.Println(r.String("{# people} liked this", n))
				}
			}
		`,
	})

	_ = initBundle(t, dir, language.English, "tokibundle", io.Discard, io.Discard)
	setOnlyMessage(t, filepath.Join(dir, "tokibundle", "catalog_en.arb"),
		"{var0, plural, offset:1 "+
			"=0 {Nobody liked this} =1 {You liked this} "+
			"one {You and # other liked this} other {You and # others liked this}}")
	_ = initBundle(t, dir, language.English, "tokibundle", io.Discard, io.Discard)

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	expect := stripLeadingSpaces(strings.TrimSpace(`
		Nobody liked this
		You liked this
		You and 1 other liked this
		You and 2 others liked this
	`))
	actual := stripLeadingSpaces(strings.TrimSpace(string(out)))
	require.Equal(t, expect, actual)
}

// TestApplyChangesAndBuild makes sure editor builds render edited
// messages and leave all files untouched when an edit isn't valid ICU.
func TestApplyChangesAndBuild(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.German)
				fmt.Println(r.String("Hello {text}!", "Welt"))
			}
		`,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate", "-l=en", "-t=de"}
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	f, err := os.Open(filepath.Join(dir, "tokibundle", "catalog_de.arb"))
	require.NoError(t, err)
	catalog, err := arb.NewDecoder().Decode(f)
	require.NoError(t, f.Close())
	require.NoError(t, err)
	require.Len(t, catalog.Messages, 1)
	var id string
	for k := range catalog.Messages {
		id = k
	}

	build := func(message string) error {
		_, err := cli.ApplyChangesAndBuild(
			osEnv(), dir, "tokibundle", language.English,
			[]editorapp.BundleEdit{{Locale: "de", TIKID: id, Message: message}},
		)
		return err
	}

	ss := snapshotFiles(t, dir)
	require.Error(t, build("Hallo {var0!"))
	ss.RequireUnchanged(t, dir)

	require.NoError(t, build("Hallo {var0}!"))

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	require.Equal(t, "Hallo Welt!", strings.TrimSpace(string(out)))
}

// TestGenerateCrossCompile makes sure the generated bundle
// compiles for targets where int and uint are 32 bits wide.
func TestGenerateCrossCompile(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, ModName)
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("integer: {integer}", int(1)))
				fmt.Println(r.String("integer: {integer}", uint(1)))
				fmt.Println(r.String("integer: {integer}", int64(1)))
				fmt.Println(r.String("integer: {integer}", uint64(1)))
				fmt.Println(r.String("searched {# files}", 2))
			}
		`,
	})

	generate := func() {
		runInDir(t, dir, func() {
			args := []string{"toki", "generate", "-l=en"}
			result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
			require.NoError(t, result.Err)
			require.Zero(t, exitCode)
		})
	}
	generate()

	// Give the plural an offset greater than the maximum uint on 32 bit targets.
	// Offsets come from the catalogs which are edited by translators.
	catalog := filepath.Join(dir, "tokibundle", "catalog_en.arb")
	contents, err := os.ReadFile(catalog)
	require.NoError(t, err)
	patched := strings.Replace(string(contents),
		"{var0, plural, other", "{var0, plural, offset:5000000000 other", 1)
	require.NotEqual(t, string(contents), patched, "plural not found in %s", catalog)
	require.NoError(t, os.WriteFile(catalog, []byte(patched), 0o644))
	generate()

	// Build for the host first to add the bundle imports to go.mod.
	requireCompiles(t, dir, "", "")

	for _, target := range []struct{ goos, goarch string }{
		{"linux", "386"},   // 32-bit int, little endian.
		{"linux", "mips"},  // 32-bit int, big endian.
		{"wasip1", "wasm"}, // 64-bit int, 32-bit pointers.
	} {
		t.Run(target.goos+"_"+target.goarch, func(t *testing.T) {
			requireCompiles(t, dir, target.goos, target.goarch)
		})
	}
}

// requireCompiles builds the main package in dir for the given target.
// Empty goos and goarch build for the host.
func requireCompiles(tb testing.TB, dir, goos, goarch string) {
	tb.Helper()

	cmd := exec.Command("go", "build", "-o", os.DevNull, ".")
	cmd.Dir, cmd.Env = dir, osEnv()
	if goos != "" {
		cmd.Env = append(cmd.Env, "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	}
	out, err := cmd.CombinedOutput()
	require.NoError(tb, err, "GOOS=%s GOARCH=%s; output: %s", goos, goarch, out)
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
					TimeNow.Format(time.RFC3339), cli.Version,
				),
				".tokidomain.yml": "# The TIK domain for this directory tree.\n" +
					"# The description provides context for translators working on this domain.\n" +
					"# Also see: https://github.com/romshark/tik/blob/main/SPECIFICATION.md#domains\n" +
					"\n" +
					"# This defines the name of the domain.\n" +
					"name: " + ModName + "\n" +
					"\n" +
					"# The description provides translators with additional context and instructions on\n" +
					"# how to translate within a directory tree, such as the expected tone, style,\n" +
					"# target audience, or terminology conventions.\n" +
					"description: \"\"\n",
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
					TimeNow.Format(time.RFC3339), cli.Version),
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
					TimeNow.Format(time.RFC3339), cli.Version),
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
					TimeNow.Format(time.RFC3339), cli.Version),
				"pkg/i18n/toki/catalog_de.arb": fmt.Sprintf(`{
					"@@locale": "de",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339), cli.Version),
				"pkg/i18n/toki/catalog_en_us.arb": fmt.Sprintf(`{
					"@@locale": "en-US",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339), cli.Version),
			},
		},
		{
			name: "remove last TIK",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				Files: map[string]string{
					"main.go": `
						package main
						import "fmt"
						import "tstmod/tokibundle"
						import "golang.org/x/text/language"
						func main() {
							r, _ := tokibundle.Match(language.English)
							fmt.Println(r.String("removed later"))
						}
					`,
				},
				FilesAfterInit: map[string]string{
					"main.go": `
						package main
						func main() {}
					`,
				},
			},
			expectFiles: map[string]string{
				"tokibundle/catalog_en.arb": fmt.Sprintf(`{
					"@@locale": "en",
					"@@last_modified": %q,
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q
				}`,
					TimeNow.Format(time.RFC3339), cli.Version),
			},
		},
		{
			// Generation fills empty native messages from their TIKs,
			// which completes the catalog.
			name: "require completeness with empty native message",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				Files: map[string]string{
					"main.go": `
						package main
						import "tstmod/tokibundle"
						func main() { print(tokibundle.Default().String("translated")) }
					`,
				},
				FilesAfterInit: map[string]string{
					"tokibundle/catalog_en.arb": `{
						"@@locale": "en",
						"msg9d21e6c2519b8be9": ""
					}`,
				},
			},
			args: []string{"-require-complete"},
			expectFiles: map[string]string{
				"tokibundle/catalog_en.arb": fmt.Sprintf(`{
					"@@locale": "en",
					"@@x-generator": "github.com/romshark/toki",
					"@@x-generator-version": %q,
					"msg9d21e6c2519b8be9": "translated",
					"@msg9d21e6c2519b8be9": {
						"type": "text"
					}
				}`, cli.Version),
			},
		},
		{
			name: "same TIK at multiple call sites",
			setup: Setup{
				InitGoMod: true,
				Files: map[string]string{
					"main.go": `
						package main
						import "fmt"
						import "tstmod/tokibundle"
						import "golang.org/x/text/language"
						func main() {
							r, _ := tokibundle.Match(language.English)
							fmt.Println(r.String("duplicated"))
							fmt.Println(r.String("duplicated"))
						}
					`,
				},
			},
			args: []string{"-l=en"},
		},
		{
			name: "same TIK at multiple call sites with ignored spaces",
			setup: Setup{
				InitGoMod: true,
				Files: map[string]string{
					"main.go": `
						package main
						import "fmt"
						import "tstmod/tokibundle"
						import "golang.org/x/text/language"
						func main() {
							r, _ := tokibundle.Match(language.English)
							fmt.Println(r.String(" duplicated "))
							fmt.Println(r.String("duplicated"))
						}
					`,
				},
			},
			args: []string{"-l=en"},
		},
		{
			name: "require completeness with empty messages of removed TIK",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				Files: map[string]string{
					"main.go": `
						package main
						import "tstmod/tokibundle"
						func main() { print(tokibundle.Default().String("translated")) }
					`,
				},
				FilesAfterInit: map[string]string{
					"tokibundle/catalog_en.arb": `{
						"@@locale": "en",
						"msg9d21e6c2519b8be9": "translated",
						"msg0123456789abcdef": ""
					}`,
					"tokibundle/catalog_de.arb": `{
						"@@locale": "de",
						"msg9d21e6c2519b8be9": "übersetzt",
						"msg0123456789abcdef": ""
					}`,
				},
			},
			args: []string{"-require-complete"},
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

// TestGenerateRequireCompleteAfterSync makes sure -require-complete judges the
// catalogs after synchronizing them with the source, not as they were loaded.
func TestGenerateRequireCompleteAfterSync(t *testing.T) {
	const mainHeader = `
		package main
		import "fmt"
		import "tstmod/tokibundle"
		import "golang.org/x/text/language"
		func main() {
			r, _ := tokibundle.Match(language.English)
	`
	generate := func(t *testing.T, dir string, args ...string) error {
		t.Helper()
		var err error
		runInDir(t, dir, func() {
			res, _ := cli.Run(
				append([]string{"toki", "generate"}, args...),
				osEnv(), io.Discard, io.Discard, TimeNow,
			)
			err = res.Err
		})
		return err
	}

	t.Run("new blank translation", func(t *testing.T) {
		dir := t.TempDir()
		initGoMod(t, dir, ModName)
		writeFiles(t, dir, map[string]string{
			"main.go": mainHeader + `fmt.Println(r.String("first")) }`,
		})
		require.NoError(t, generate(t, dir, "-l=en", "-t=de"))
		setOnlyMessage(t, filepath.Join(dir, "tokibundle", "catalog_de.arb"), "erste")
		require.NoError(t, generate(t, dir, "-require-complete"))

		writeFiles(t, dir, map[string]string{
			"main.go": mainHeader + `
				fmt.Println(r.String("first"))
				fmt.Println(r.String("second"))
			}`,
		})
		require.ErrorIs(t, generate(t, dir, "-require-complete"), cli.ErrBundleIncomplete)
	})

	t.Run("obsolete incomplete translation", func(t *testing.T) {
		dir := t.TempDir()
		initGoMod(t, dir, ModName)
		writeFiles(t, dir, map[string]string{
			"main.go": mainHeader + `fmt.Println(r.String("{# apples}", 2)) }`,
		})
		require.NoError(t, generate(t, dir, "-l=en", "-t=de"))
		setOnlyMessage(t, filepath.Join(dir, "tokibundle", "catalog_en.arb"),
			"{var0, plural, one {# apple} other {# apples}}")
		// German requires the "one" option too.
		setOnlyMessage(t, filepath.Join(dir, "tokibundle", "catalog_de.arb"),
			"{var0, plural, other {# Äpfel}}")
		require.ErrorIs(t, generate(t, dir, "-require-complete"), cli.ErrBundleIncomplete)

		writeFiles(t, dir, map[string]string{
			"main.go": "package main\nfunc main() {}",
		})
		require.NoError(t, generate(t, dir, "-require-complete"))
	})
}

// TestGenerateModPath makes sure -m generates a module
// outside the module of the working directory.
func TestGenerateModPath(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, ModName)
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("hello"))
			}
		`,
	})
	other := t.TempDir()
	initGoMod(t, other, "othermod")

	var result cli.Result
	runInDir(t, other, func() {
		var exitCode int
		result, exitCode = cli.Run(
			[]string{"toki", "generate", "-l=en", "-m=" + dir},
			osEnv(), io.Discard, io.Discard, TimeNow,
		)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})
	require.Equal(t, 1, result.Scan.Texts.Len())
	// -trimpath trims the module directory.
	require.Equal(t, "/main.go", result.Scan.Texts.At(0).Position.Filename)
	require.NoDirExists(t, filepath.Join(other, "tokibundle"))

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %q", string(out))
	require.Equal(t, "hello", strings.TrimSpace(string(out)))
}

// TestGenerateJSONPackageError makes sure JSON output reports errors
// that stop the source scan.
func TestGenerateJSONPackageError(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, ModName)
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			func main() {
		`,
	})

	var result cli.Result
	runInDir(t, dir, func() {
		var exitCode int
		result, exitCode = cli.Run(
			[]string{"toki", "generate", "-l=en", "-json"},
			osEnv(), io.Discard, io.Discard, TimeNow,
		)
		require.NotZero(t, exitCode)
	})
	require.ErrorIs(t, result.Err, cli.ErrAnalyzingSource)
	require.Nil(t, result.Scan)

	var stdout bytes.Buffer
	result.Print(&stdout)
	var out cli.ResultJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), stdout.String())
	require.Equal(t, result.Err.Error(), out.Error)
}

// TestGenerateDomainFileNotOverwritten verifies that an existing .tokidomain.yml file
// is preserved when running `toki generate` again.
func TestGenerateDomainFileNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")

	customDomain := "name: custom\ndescription: my custom domain"
	writeFiles(t, dir, map[string]string{
		".tokidomain.yml": customDomain,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate", "-l=en"}
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	// Verify .tokidomain.yml was not overwritten.
	content, err := os.ReadFile(filepath.Join(dir, ".tokidomain.yml"))
	require.NoError(t, err)
	require.Equal(t, customDomain, string(content))
}

// TestGenerateSameTextDifferentDomains verifies that TIKs in different domains
// (including parent vs. sub-domain) can share a text when their contexts differ.
func TestGenerateSameTextDifferentDomains(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, "tstmod")
	writeFiles(t, dir, map[string]string{
		".tokidomain.yml": "name: root\ndescription: \"\"",
		"main.go": `
			package main
			import "fmt"
			import "tstmod/sub"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("hello world"), sub.Hello())
			}
		`,
		"sub/.tokidomain.yml": "name: sub\ndescription: \"\"",
		"sub/sub.go": `
			package sub
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func Hello() string {
				r, _ := tokibundle.Match(language.English)
				return r.String("[sub] hello world")
			}
		`,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate", "-l=en"}
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	requireRunOutput(t, dir, ".", "hello world hello world")
}

// TestGenerateSimilarlyNamedPackage verifies that a package whose path ends
// in the bundle package name is scanned and isn't mistaken for the bundle.
func TestGenerateSimilarlyNamedPackage(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, ModName)
	_ = initBundle(t, dir, language.English, "tokibundle", io.Discard, io.Discard)
	writeFiles(t, dir, map[string]string{
		"main.go": `
			package main
			import "fmt"
			import "tstmod/internal/mytokibundle"
			import "tstmod/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(mytokibundle.Greet(r))
			}
		`,
		"internal/mytokibundle/mytokibundle.go": `
			package mytokibundle
			import "tstmod/tokibundle"
			func Greet(r tokibundle.Reader) string { return r.String("hello") }
		`,
	})

	runInDir(t, dir, func() {
		args := []string{"toki", "generate"}
		result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
		require.NoError(t, result.Err)
		require.Zero(t, exitCode)
	})

	requireRunOutput(t, dir, ".", "hello")
}

// TestGenerateBundlesWithSameName verifies that -b selects the bundle package
// by its path when several bundles in a module share a package name.
func TestGenerateBundlesWithSameName(t *testing.T) {
	dir := t.TempDir()
	initGoMod(t, dir, ModName)
	_ = initBundle(t, dir, language.English, "app1/tokibundle", io.Discard, io.Discard)
	_ = initBundle(t, dir, language.English, "app2/tokibundle", io.Discard, io.Discard)
	writeFiles(t, dir, map[string]string{
		"app1/main.go": `
			package main
			import "fmt"
			import "tstmod/app1/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("app one"))
			}
		`,
		"app2/main.go": `
			package main
			import "fmt"
			import "tstmod/app2/tokibundle"
			import "golang.org/x/text/language"
			func main() {
				r, _ := tokibundle.Match(language.English)
				fmt.Println(r.String("app two"))
			}
		`,
	})

	runInDir(t, dir, func() {
		for _, bundle := range []string{"app1/tokibundle", "app2/tokibundle"} {
			args := []string{"toki", "generate", "-b", bundle}
			result, exitCode := cli.Run(args, osEnv(), io.Discard, io.Discard, TimeNow)
			require.NoError(t, result.Err, bundle)
			require.Zero(t, exitCode, bundle)
		}
	})

	requireRunOutput(t, dir, "./app1", "app one")
	requireRunOutput(t, dir, "./app2", "app two")
}

// requireRunOutput runs the main package pkg in dir and requires it to print expect.
func requireRunOutput(tb testing.TB, dir, pkg, expect string) {
	tb.Helper()

	cmd := exec.Command("go", "run", pkg)
	cmd.Dir, cmd.Env = dir, osEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(tb, err, "go run %s; output: %s", pkg, out)
	require.Equal(tb, expect, strings.TrimSpace(string(out)))
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
					"tokibundle/" + cli.MainBundleFileGo: "this file is broken",
				},
			},
			args:           []string{"-l=en"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, cli.ErrAnalyzingSource)
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
				require.ErrorIs(tt, err, cli.ErrAnalyzingSource)
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
				require.ErrorIs(tt, err, cli.ErrMissingLocaleParam)
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
				require.ErrorIs(tt, err, cli.ErrInvalidCLIArgs)
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
				require.ErrorIs(tt, err, cli.ErrInvalidCLIArgs)
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
				require.ErrorIs(tt, err, cli.ErrInvalidCLIArgs)
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
				require.ErrorIs(tt, err, cli.ErrInvalidCLIArgs)
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
							"msg9d21e6c2519b8be9": "übersetzt",
							"@msg9d21e6c2519b8be9": {
								"type": "text"
							}
						}
					`, cli.Version),
				},
			},
			args:           []string{"-l=en"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, cli.ErrAnalyzingSource)
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
				require.ErrorIs(tt, err, cli.ErrBundleIncomplete)
				require.Equal(t, "bundle contains incomplete catalogs", err.Error())
			},
		},
		{
			name: "require completeness with empty translation",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				Files: map[string]string{
					"main.go": `
						package main
						import "tstmod/tokibundle"
						func main() { print(tokibundle.Default().String("translated")) }
					`,
				},
				FilesAfterInit: map[string]string{
					"tokibundle/catalog_de.arb": `{
						"@@locale": "de",
						"msg9d21e6c2519b8be9": ""
					}`,
				},
			},
			args:           []string{"-require-complete"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, cli.ErrBundleIncomplete)
			},
		},
		{
			name: "require completeness with message missing in catalog",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				Files: map[string]string{
					"main.go": `
						package main
						import "tstmod/tokibundle"
						func main() { print(tokibundle.Default().String("translated")) }
					`,
				},
				FilesAfterInit: map[string]string{
					"tokibundle/catalog_de.arb": `{"@@locale": "de"}`,
				},
			},
			args:           []string{"-require-complete"},
			expectExitCode: 1,
			expectErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorIs(tt, err, cli.ErrBundleIncomplete)
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
					require.ErrorAs(t, err, &tik.ParseError{})
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
		{
			name: "ERR same TIK in different domains",
			setup: Setup{
				InitGoMod: true, InitBundle: true,
				FilesAfterInit: map[string]string{
					"main.go": `
					package main
					import "tstmod/tokibundle"
					func main() { print(tokibundle.Default().String("hello world")) }
					`,
					"sub/.tokidomain.yml": "name: sub",
					"sub/sub.go": `
					package sub
					import "tstmod/tokibundle"
					func Hello() string { return tokibundle.Default().String("hello world") }
					`,
				},
			},
			expectSrcErrs: []SourceError{
				{"sub.go:3:35", func(tt require.TestingT, err error, i ...any) {
					require.ErrorIs(tt, err, codeparse.ErrTIKInMultipleDomains)
					require.ErrorContains(tt, err, `"hello world" (also at `)
					require.ErrorContains(tt, err, "main.go:3:26)")
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, resLint, resGenerate := tt.setup.generate(t, TimeNow, tt.args...)
			check := func(t *testing.T, res RunResult) {
				t.Helper()
				require.ErrorIs(t, res.Err, cli.ErrSourceErrors)
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
		res, exitCode := cli.Run(args, osEnv(), os.Stderr, os.Stdout, TimeNow)
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

func runInDir(tb testing.TB, dir string, fn func()) {
	tb.Helper()
	wd, err := os.Getwd()
	require.NoError(tb, err)
	defer func() { _ = os.Chdir(wd) }()
	require.NoError(tb, os.Chdir(dir))
	fn()
}

func initBundle(
	tb testing.TB, dir string, locale language.Tag, bundlePkg string,
	stderr, stdout io.Writer,
) (result cli.Result) {
	tb.Helper()

	runInDir(tb, dir, func() {
		var exitCode int
		result, exitCode = cli.Run([]string{
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

// setOnlyMessage sets the ICU message of the only message in the catalog file at path.
func setOnlyMessage(tb testing.TB, path, icuMessage string) {
	tb.Helper()
	f, err := os.Open(path)
	require.NoError(tb, err)
	catalog, err := arb.NewDecoder().Decode(f)
	require.NoError(tb, f.Close())
	require.NoError(tb, err)
	require.Len(tb, catalog.Messages, 1)
	for id, msg := range catalog.Messages {
		msg.ICUMessage = icuMessage
		catalog.Messages[id] = msg
	}
	var buf bytes.Buffer
	require.NoError(tb, arb.Encode(&buf, catalog, "\t"))
	require.NoError(tb, os.WriteFile(path, buf.Bytes(), 0o644))
}

type Setup struct {
	Stderr, Stdout io.Writer
	InitGoMod      bool
	InitBundle     bool
	Files          map[string]string
	FilesAfterInit map[string]string
}

type RunResult struct {
	cli.Result
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
		generateResult.Result, generateResult.ExitCode = cli.Run(
			a, osEnv(), stderr, stdout, now,
		)

		ss := snapshotFiles(tb, dir)
		a = append([]string{"toki", "lint"}, args...)
		lintResult.Result, lintResult.ExitCode = cli.Run(
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
		current, ok := after.m[path]
		if !ok {
			tb.Errorf("file unexpectedly deleted: %s", path)
			continue
		}
		if !bytes.Equal(old, current) {
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
	// go test sets GOFLAGS to "-mod=readonly" which stops nested go commands
	// from adding the imports of the generated files to go.mod.
	// "-mod=mod" lets them add those imports.
	return append(os.Environ(), "GOFLAGS=-mod=mod")
}
