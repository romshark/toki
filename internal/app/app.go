package app

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
)

var (
	ErrGenerateBundleFirst = errors.New("first generate a bundle using `toki generate`")
	ErrSourceErrors        = errors.New("source code contains errors")
	ErrNoCommand           = errors.New("no command")
	ErrUnknownCommand      = errors.New("unknown command")
	ErrAnalyzingSource     = errors.New("analyzing sources")
	ErrInvalidCLIArgs      = errors.New("invalid arguments")
	ErrMissingLocaleParam  = errors.New(
		"please provide a valid non-und BCP 47 locale for the default language of your " +
			"original code base using the 'l' parameter",
	)
	ErrBundleIncomplete = errors.New("bundle contains incomplete catalogs")
)

const Version = "0.8.0"

const MainBundleFileGo = "bundle_gen.go"

func Run(
	osArgs, env []string, stderr, stdout io.Writer, now time.Time,
) (result Result, exitCode int) {
	if len(osArgs) < 2 {
		return Result{
			Err: fmt.Errorf("%w, use either of: [generate,lint]", ErrNoCommand),
		}, 2
	}

	switch osArgs[1] {
	case "version":
		return Result{}, printVersionInfoAndExit(stderr, stdout)

	case "lint", "generate":
		g := Generate{
			hasher:           xxhash.New(),
			icuTokenizer:     new(icumsg.Tokenizer),
			tikParser:        tik.NewParser(tik.DefaultConfig),
			tikICUTranslator: tik.NewICUTranslator(tik.DefaultConfig),
		}
		lintOnly := osArgs[1] == "lint"
		r := g.Run(osArgs, env, lintOnly, stderr, now)
		switch {
		case errors.Is(r.Err, ErrInvalidCLIArgs):
			return r, 2
		case r.Err != nil:
			return r, 1
		}
		return r, 0
	case "webedit":
		w := WebEdit{
			hasher:           xxhash.New(),
			icuTokenizer:     new(icumsg.Tokenizer),
			tikParser:        tik.NewParser(tik.DefaultConfig),
			tikICUTranslator: tik.NewICUTranslator(tik.DefaultConfig),
		}
		err := w.Run(osArgs, env, stderr)
		switch {
		case errors.Is(err, ErrInvalidCLIArgs):
			return Result{Err: err}, 2
		case err != nil:
			return Result{Err: err}, 1
		}
		return Result{}, 0
	}
	return Result{
		Err: fmt.Errorf("%w %q, use either of: [generate,lint,webedit]",
			ErrUnknownCommand, osArgs[1]),
	}, 2
}

func printVersionInfoAndExit(stderr, stdout io.Writer) (exitCode int) {
	p, err := exec.LookPath(os.Args[0])
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "resolving executable file path: %v\n", err)
		return 1
	}

	f, err := os.Open(p)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "opening executable file %q: %v\n", os.Args[0], err)
		return 1
	}

	info, err := buildinfo.Read(f)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "reading build information: %v\n", err)
	}

	_, _ = fmt.Fprintf(stdout, "Toki v%s\n\n", Version)
	_, _ = fmt.Fprintf(stdout, "%v\n", info)
	return 0
}
