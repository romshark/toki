package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/romshark/toki/internal/tokibundle"
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

// command is a command of toki in the usage text.
type command struct{ name, synopsis string }

// commands lists the commands for the usage text, described in the language of tr.
func commands(tr tokibundle.Reader) []command {
	return []command{
		// The synopsis of toki generate in the list of commands.
		{"generate", tr.String("Extract texts from the source code and generate the localization bundle")},
		// The synopsis of toki lint in the list of commands.
		{"lint", tr.String("Check the source code and the catalogs without writing any files")},
		// The synopsis of toki edit in the list of commands.
		{"edit", tr.String("Open the translation editor")},
		// The synopsis of toki repair in the list of commands.
		{"repair", tr.String("Rewrite native-locale messages that disagree with their TIK")},
		// The synopsis of toki version in the list of commands.
		{"version", tr.String("Print version information")},
		// The synopsis of toki help in the list of commands.
		{"help", tr.String("Print the usage of a command")},
	}
}

// Run runs the command of osArgs. It writes its text for the user in the
// language of tr. The output of -json, which programs read, is English.
func Run(
	osArgs, env []string, tr tokibundle.Reader, stderr, stdout io.Writer, now time.Time,
) (result Result, exitCode int) {
	if len(osArgs) < 2 {
		printUsage(tr, stderr)
		return Result{}, 2
	}

	if isHelp(osArgs[1]) {
		if len(osArgs) < 3 || isHelp(osArgs[2]) {
			printUsage(tr, stdout)
			return Result{}, 0
		}
		// `toki help <command>` is the same as `toki <command> -h`.
		osArgs = []string{osArgs[0], osArgs[2], "-h"}
	}

	switch osArgs[1] {
	case "version", "-version", "--version":
		fs := newFlagSet("version")
		// The usage of a flag of toki version.
		full := fs.Bool("full", false, tr.String(
			"also print the build information with the module dependencies"))
		if err := flagError(tr, fs, stdout, fs.Parse(osArgs[2:])); err != nil {
			return exit(Result{Err: err})
		}
		printVersion(stdout, *full)
		return Result{}, 0

	case "repair":
		return exit(Result{Err: new(Repair).Run(osArgs, env, tr, stderr, stdout)})

	case "lint", "generate":
		g := Generate{
			hasher:           xxhash.New(),
			icuTokenizer:     new(icumsg.Tokenizer),
			tikParser:        tik.NewParser(tik.DefaultConfig),
			tikICUTranslator: tik.NewICUTranslator(tik.DefaultConfig),
		}
		lintOnly := osArgs[1] == "lint"
		return exit(g.Run(osArgs, env, tr, lintOnly, stderr, stdout, now))

	case "edit":
		return exit(Result{Err: new(Edit).Run(osArgs, env, tr, stderr, stdout)})
	}
	return Result{Err: &UnknownCommandError{Command: osArgs[1]}}, 2
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

// printUsage prints the usage of toki to w in the language of tr.
func printUsage(tr tokibundle.Reader, w io.Writer) {
	var b strings.Builder
	// The first line of the usage of toki.
	b.WriteString(tr.String("Toki is an i18n framework for Go."))
	b.WriteString("\n\n")
	// The heading of the usage line, as in "Usage: toki <command> [flags]".
	b.WriteString(tr.String("Usage:"))
	b.WriteString("\n\n\ttoki <command> [flags]\n\n")
	// The heading of the list of commands.
	b.WriteString(tr.String("Commands:"))
	b.WriteString("\n\n")
	for _, c := range commands(tr) {
		_, _ = fmt.Fprintf(&b, "\t%-10s %s\n", c.name, c.synopsis)
	}
	b.WriteString("\n")
	// The last line of the usage of toki. Keep toki help <command>, a command.
	b.WriteString(tr.String("Run 'toki help <command>' for the usage of a command."))
	b.WriteString("\n")
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
// to stdout in the language of tr and returns [flag.ErrHelp]. It wraps any
// other error in [ErrInvalidCLIArgs].
func flagError(tr tokibundle.Reader, fs *flag.FlagSet, stdout io.Writer, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, flag.ErrHelp):
		printCommandUsage(tr, stdout, fs)
		return flag.ErrHelp
	}
	return fmt.Errorf("%w: %w", ErrInvalidCLIArgs, err)
}

// printCommandUsage prints the usage of the command of fs to w in the
// language of tr.
func printCommandUsage(tr tokibundle.Reader, w io.Writer, fs *flag.FlagSet) {
	var synopsis string
	for _, c := range commands(tr) {
		if c.name == fs.Name() {
			synopsis = c.synopsis
			break
		}
	}
	var hasFlags bool
	fs.VisitAll(func(*flag.Flag) { hasFlags = true })
	if !hasFlags {
		// The usage line of a command without flags. The placeholder is the
		// command, as version.
		_, _ = fmt.Fprintf(w, "%s\n\n%s.\n", tr.String("Usage: toki {text}", fs.Name()), synopsis)
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n\n%s.\n\n%s\n",
		// The usage line of a command. The placeholder is the command, as
		// generate. Keep [flags], which stands for its flags.
		tr.String("Usage: toki {text} [flags]", fs.Name()), synopsis,
		// The heading of the list of the flags of a command.
		tr.String("Flags:"))
	printFlags(tr, w, fs)
}

// printFlags prints the flags of fs to w as [flag.FlagSet.PrintDefaults] does,
// in the language of tr.
func printFlags(tr tokibundle.Reader, w io.Writer, fs *flag.FlagSet) {
	var b strings.Builder
	fs.VisitAll(func(f *flag.Flag) {
		b.WriteString("  -")
		b.WriteString(f.Name)
		name, usage := flag.UnquoteUsage(f)
		if name != "" {
			b.WriteByte(' ')
			b.WriteString(name)
		}
		if len(f.Name) == 1 && name == "" {
			b.WriteByte('\t') // A boolean flag of 1 letter keeps its usage on its line.
		} else {
			b.WriteString("\n    \t")
		}
		b.WriteString(strings.ReplaceAll(usage, "\n", "\n    \t"))
		if def := defaultValue(f); def != "" {
			b.WriteByte(' ')
			// Follows the usage of a flag. The placeholder is its default value,
			// as "tokibundle" or true.
			b.WriteString(tr.String("(default {text})", def))
		}
		b.WriteByte('\n')
	})
	_, _ = io.WriteString(w, b.String())
}

// defaultValue returns the default value of f as the usage shows it, quoted
// for a string, and "" for the zero value, which the usage leaves out.
func defaultValue(f *flag.Flag) string {
	switch f.DefValue {
	case "", "false", "0":
		return ""
	}
	if g, ok := f.Value.(flag.Getter); ok {
		if _, ok := g.Get().(string); ok {
			return strconv.Quote(f.DefValue)
		}
	}
	return f.DefValue
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
