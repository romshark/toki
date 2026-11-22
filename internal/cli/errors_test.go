package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/tokibundle"
	"github.com/romshark/toki/internal/uilang"
)

// TestErrorTextEnglish makes sure that the English text of each error that
// errorText translates is the text of the error, which the logs showed
// before the command line had translations and -json still shows.
func TestErrorTextEnglish(t *testing.T) {
	en := tokibundle.Default()
	for _, err := range []error{
		&UnknownCommandError{Command: "nope"},
		&BundleCorruptError{Corrupt: 3},
		&LocaleMismatchError{Flag: "de", Bundle: "en", BundlePkg: "tokibundle"},
		fmt.Errorf("%w: %w", ErrInvalidCLIArgs, errors.New("flag provided but not defined: -x")),
		fmt.Errorf("%w: %w", ErrInvalidCLIArgs,
			&config.LocaleFlagError{Flag: "l", Value: "xx", Err: errors.New("not well-formed")}),
		fmt.Errorf("%w: %w", ErrInvalidCLIArgs, &config.LocaleFlagError{Flag: "t", Value: "und"}),
		fmt.Errorf("%w: %w", ErrAnalyzingSource, errors.New("loading packages")),
		ErrMissingLocaleParam,
		ErrSourceErrors,
		ErrBundleIncomplete,
		ErrDesktopUnavailable,
	} {
		require.Equal(t, err.Error(), errorText(en, err))
	}
}

func TestErrorTextTranslates(t *testing.T) {
	de := uilang.Reader(language.German)
	err := &UnknownCommandError{Command: "nope"}
	require.NotEqual(t, err.Error(), errorText(de, err))
	require.Contains(t, errorText(de, err), "nope")

	// An error of another layer stays English.
	other := errors.New("permission denied")
	require.Equal(t, other.Error(), errorText(de, other))
}
