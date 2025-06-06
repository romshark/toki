package app

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
)

var (
	ErrSourceErrors       = errors.New("source code contains errors")
	ErrNoCommand          = errors.New("no command")
	ErrUnknownCommand     = errors.New("unknown command")
	ErrAnalyzingSource    = errors.New("analyzing sources")
	ErrInvalidCLIArgs     = errors.New("invalid arguments")
	ErrMissingLocaleParam = errors.New(
		"please provide a valid BCP 47 locale for the default language of your " +
			"original code base using the 'l' parameter",
	)
)

const Version = "0.5.2"

const MainBundleFileGo = "bundle_gen.go"

func Run(osArgs []string, now time.Time) (result Result, exitCode int) {
	if len(osArgs) < 2 {
		return Result{
			Err: fmt.Errorf("%w, use either of: [generate,lint]", ErrNoCommand),
		}, 2
	}

	switch osArgs[1] {
	case "version":
		printVersionInfoAndExit()

	case "lint", "generate":
		g := Generate{
			hasher:           xxhash.New(),
			icuTokenizer:     new(icumsg.Tokenizer),
			tikParser:        tik.NewParser(defaultTIKConfig),
			tikICUTranslator: tik.NewICUTranslator(defaultTIKConfig),
		}
		lintOnly := osArgs[1] == "lint"
		r := g.Run(osArgs, lintOnly, now)
		switch {
		case errors.Is(r.Err, ErrInvalidCLIArgs):
			return r, 2
		case r.Err != nil:
			return r, 1
		}
		return r, 0
	}
	return Result{
		Err: fmt.Errorf("%w %q, use either of: [generate,lint]",
			ErrUnknownCommand, osArgs[1]),
	}, 2
}

func printVersionInfoAndExit() {
	defer os.Exit(0)

	p, err := exec.LookPath(os.Args[0])
	if err != nil {
		fmt.Printf("resolving executable file path: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Open(p)
	if err != nil {
		fmt.Printf("opening executable file %q: %v\n", os.Args[0], err)
		os.Exit(1)
	}

	info, err := buildinfo.Read(f)
	if err != nil {
		fmt.Printf("reading build information: %v\n", err)
	}

	fmt.Printf("Toki v%s\n\n", Version)
	fmt.Printf("%v\n", info)
}

var defaultTIKConfig = tik.DefaultConfig()
