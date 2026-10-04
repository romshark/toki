package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"

	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/bundlerepair"
	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/log"
)

// Repair implements the command `toki repair`.
//
// Repair only fixes *corruption* — native-locale ARB messages that disagree
// with the TIK in source code (locked-ICU mismatch or placeholder-metadata
// mismatch). It does NOT add missing entries; that is `toki generate`'s job.
// If the bundle is already consistent, Repair is a no-op.
type Repair struct{}

func (e *Repair) Run(osArgs, env []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("repair", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundlePkg := fs.String("b", "tokibundle", "path to generated Go bundle package")
	if err := fs.Parse(osArgs[2:]); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCLIArgs, err)
	}

	log.SetWriter(stderr, false)

	modDir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	n, err := RepairBundle(env, modDir, *bundlePkg)
	if err != nil {
		return err
	}
	if n == 0 {
		log.Info("bundle is consistent, nothing to repair")
		return nil
	}
	log.Info("repair complete", slog.Int("repaired", n))
	return nil
}

// RepairBundle fixes corrupt native-locale ARB entries by regenerating
// them from the TIK source, then writes the updated ARB and the Go bundle
// back to disk. Returns the number of messages repaired (zero when the
// bundle is already consistent). Missing entries are ignored — run
// `toki generate` (or [GenerateBundle]) to add them.
//
// modDir is the absolute path to the Go module root.
// bundlePkgPath is the bundle package path relative to modDir.
func RepairBundle(env []string, modDir, bundlePkgPath string) (int, error) {
	absBundlePkg := filepath.Join(modDir, bundlePkgPath)

	// We may be about to parse source that references stale generated Go
	// code — clean it first so `go list` / packages.Load succeeds. The
	// default locale is recovered from the scan below.
	parser := codeparse.NewParser(
		xxhash.New(),
		tik.NewParser(tik.DefaultConfig),
		tik.NewICUTranslator(tik.DefaultConfig),
	)

	// Pre-clean using whatever locale the existing bundle advertises. We
	// can't know it until we parse once, so we do a best-effort parse
	// first; if it fails because the generated bundle is broken, fall
	// back to cleaning blindly with the default language tag guess.
	scan, err := parser.Parse(env, modDir, "./...", bundlePkgPath, false)
	if err != nil {
		return 0, fmt.Errorf("analyzing source: %w", err)
	}

	icuTokenizer := new(icumsg.Tokenizer)
	tikICUTranslator := tik.NewICUTranslator(tik.DefaultConfig)
	repaired := bundlerepair.Repair(scan, tikICUTranslator, icuTokenizer)
	if len(repaired) == 0 {
		return 0, nil
	}

	// Write the repaired native ARB.
	for c := range scan.Catalogs.SeqRead() {
		if c.ARB.Locale != scan.DefaultLocale {
			continue
		}
		f, err := os.OpenFile(c.ARBFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return 0, fmt.Errorf("writing ARB: %w", err)
		}
		err = arb.Encode(f, c.ARB, "\t")
		_ = f.Close()
		if err != nil {
			return 0, fmt.Errorf("encoding ARB: %w", err)
		}
		break
	}

	// Regenerate Go code from the repaired scan so the bundle stays in sync.
	if err := GenerateBundle(absBundlePkg, scan); err != nil {
		return 0, fmt.Errorf("generating Go bundle: %w", err)
	}
	return len(repaired), nil
}

// ErrBundleCorrupt is returned by commands that require a consistent bundle
// when they detect corrupt native-locale entries. Point users at `toki repair`.
var ErrBundleCorrupt = errors.New(
	"bundle contains corrupt native-locale messages — run `toki repair`",
)
