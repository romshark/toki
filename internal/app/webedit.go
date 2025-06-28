package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/log"
	"github.com/romshark/toki/internal/webedit"

	"github.com/cespare/xxhash/v2"
	"github.com/romshark/icumsg"
	"github.com/romshark/tik/tik-go"
)

// WebEdit implements the command `toki webedit`.
type WebEdit struct {
	hasher           *xxhash.Digest
	icuTokenizer     *icumsg.Tokenizer
	tikParser        *tik.Parser
	tikICUTranslator *tik.ICUTranslator
}

func (g *WebEdit) Run(osArgs, env []string, stderr io.Writer) error {
	conf, err := config.ParseCLIArgsWeb(osArgs)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCLIArgs, err)
	}

	log.SetWriter(stderr, false)

	s := webedit.NewServer(conf.Host, func() (*codeparse.Scan, error) {
		mainBundleFile := filepath.Join(conf.BundlePkgPath, MainBundleFileGo)
		switch _, err := os.Stat(mainBundleFile); {
		case errors.Is(err, os.ErrNotExist):
			return nil, ErrGenerateBundleFirst
		case err != nil:
			return nil, fmt.Errorf(
				"checking main bundle file %q: %w",
				mainBundleFile, err,
			)
		}

		parser := codeparse.NewParser(g.hasher, g.tikParser, g.tikICUTranslator)

		// TODO: avoid hardcoding trimpath.
		scan, err := parser.Parse(env, "./...", conf.BundlePkgPath, false)
		if err != nil {
			err = fmt.Errorf("%w: %w", ErrAnalyzingSource, err)
			return nil, err
		}
		if scan.SourceErrors.Len() > 0 {
			return nil, ErrSourceErrors
		}
		return scan, nil
	})
	if err := s.Init(); err != nil {
		return fmt.Errorf("initializing server: %w", err)
	}

	go func() {
		log.Info("listening", slog.String("host", conf.Host))
		if err := s.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Error("listening and serving HTTP", err)
			}
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	<-ctx.Done() // Wait for interrupt signal.
	log.Info("server shutting down")
	if err := s.Shutdown(context.Background()); err != nil {
		log.Error("shutting down server", err)
	}

	return nil
}
