package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

var (
	ErrGenerateBundleFirst = errors.New("first generate a bundle using `toki generate`")
	ErrSourceErrors        = errors.New("source code contains errors")
	ErrUnknownCommand      = errors.New("unknown command")
	ErrAnalyzingSource     = errors.New("analyzing sources")
	ErrInvalidCLIArgs      = errors.New("invalid arguments")
	ErrMissingLocaleParam  = errors.New(
		"please provide a valid non-und BCP 47 locale for the default language of your " +
			"original code base using the 'l' parameter",
	)
	ErrBundleIncomplete = errors.New("bundle contains incomplete catalogs")
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

const MainBundleFileGo = "bundle_gen.go"

// commands lists the commands for the usage text.
var commands = []struct{ name, synopsis string }{
	{"generate", "Extract texts from the source code and generate the localization bundle"},
	{"lint", "Check the source code and the catalogs without writing any files"},
	{"edit", "Open the translation editor"},
	{"repair", "Rewrite native-locale messages that disagree with their TIK"},
	{"version", "Print version information"},
	{"help", "Print the usage of a command"},
}

func Run(
	osArgs, env []string, stderr, stdout io.Writer, now time.Time,
) (result Result, exitCode int) {
	if len(osArgs) < 2 {
		printUsage(stderr)
		return Result{}, 2
	}

	if isHelp(osArgs[1]) {
		if len(osArgs) < 3 || isHelp(osArgs[2]) {
			printUsage(stdout)
			return Result{}, 0
		}
		// `toki help <command>` is the same as `toki <command> -h`.
		osArgs = []string{osArgs[0], osArgs[2], "-h"}
	}

	switch osArgs[1] {
	case "version", "-version", "--version":
		fs := newFlagSet("version")
		full := fs.Bool("full", false,
			"also print the build information with the module dependencies")
		if err := flagError(fs, stdout, fs.Parse(osArgs[2:])); err != nil {
			return exit(Result{Err: err})
		}
		printVersion(stdout, *full)
		return Result{}, 0

	case "repair":
		return exit(Result{Err: new(Repair).Run(osArgs, env, stderr, stdout)})

	case "lint", "generate":
		g := Generate{
			hasher:           xxhash.New(),
			icuTokenizer:     new(icumsg.Tokenizer),
			tikParser:        tik.NewParser(tik.DefaultConfig),
			tikICUTranslator: tik.NewICUTranslator(tik.DefaultConfig),
		}
		lintOnly := osArgs[1] == "lint"
		return exit(g.Run(osArgs, env, lintOnly, stderr, stdout, now))

	case "edit":
		return exit(Result{Err: new(Edit).Run(osArgs, env, stderr, stdout)})
	}
	return Result{
		Err: fmt.Errorf("%w %q, run 'toki help' for a list of commands",
			ErrUnknownCommand, osArgs[1]),
	}, 2
}

// exit returns r with the exit code of its error: 0 for none or a help
// request, 2 for invalid arguments and 1 for any other error.
func exit(r Result) (Result, int) {
	switch {
	case r.Err == nil:
		return r, 0
	case errors.Is(r.Err, flag.ErrHelp):
		return Result{}, 0
	case errors.Is(r.Err, ErrInvalidCLIArgs):
		return r, 2
	}
	return r, 1
}

func isHelp(arg string) bool {
	switch arg {
	case "help", "-h", "-help", "--help":
		return true
	}
	return false
}

func printUsage(w io.Writer) {
	var b strings.Builder
	b.WriteString("Toki is an i18n framework for Go.\n\n" +
		"Usage:\n\n\ttoki <command> [flags]\n\nCommands:\n\n")
	for _, c := range commands {
		_, _ = fmt.Fprintf(&b, "\t%-10s %s\n", c.name, c.synopsis)
	}
	b.WriteString("\nRun 'toki help <command>' for the usage of a command.\n")
	_, _ = io.WriteString(w, b.String())
}

// newFlagSet returns a flag set for the named command that prints nothing
// itself. Pass the error of its Parse to flagError.
func newFlagSet(command string) *flag.FlagSet {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// flagError returns the error of the command fs parses arguments for given err,
// the error of parsing them. On a help request it prints the usage of the command
// to stdout and returns [flag.ErrHelp]. It wraps any other error in
// [ErrInvalidCLIArgs].
func flagError(fs *flag.FlagSet, stdout io.Writer, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, flag.ErrHelp):
		printCommandUsage(stdout, fs)
		return flag.ErrHelp
	}
	return fmt.Errorf("%w: %w", ErrInvalidCLIArgs, err)
}

func printCommandUsage(w io.Writer, fs *flag.FlagSet) {
	var synopsis string
	for _, c := range commands {
		if c.name == fs.Name() {
			synopsis = c.synopsis
			break
		}
	}
	var hasFlags bool
	fs.VisitAll(func(*flag.Flag) { hasFlags = true })
	if !hasFlags {
		_, _ = fmt.Fprintf(w, "Usage: toki %s\n\n%s.\n", fs.Name(), synopsis)
		return
	}
	_, _ = fmt.Fprintf(w, "Usage: toki %s [flags]\n\n%s.\n\nFlags:\n",
		fs.Name(), synopsis)
	fs.SetOutput(w)
	fs.PrintDefaults()
}

// printVersion prints the version of Toki and Go to w. With full it adds the
// build information, including the module dependencies.
func printVersion(w io.Writer, full bool) {
	_, _ = fmt.Fprintf(w, "Toki %s\n", versionTag())
	if Commit != "" {
		_, _ = fmt.Fprintf(w, "commit: %s\n", Commit)
	}
	if Date != "" {
		_, _ = fmt.Fprintf(w, "built:  %s\n", Date)
	}
	_, _ = fmt.Fprintf(w, "go:     %s %s/%s\n",
		runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if !full {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		_, _ = fmt.Fprintf(w, "\n%s", info)
	}
}

// versionTag returns Version with the "v" that goreleaser leaves out.
func versionTag() string {
	if len(Version) > 0 && Version[0] >= '0' && Version[0] <= '9' {
		return "v" + Version
	}
	return Version
}

// releaseVersion returns the release of this build, as "v1.2.3",
// or "" for a build that isn't a release, such as one from a pseudo-version.
func releaseVersion() string {
	v := versionTag()
	if !semver.IsValid(v) || semver.Build(v) != "" || module.IsPseudoVersion(v) {
		return ""
	}
	return v
}
