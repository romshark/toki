package cli

import (
	"errors"
	"fmt"

	"github.com/romshark/toki/internal/config"
	"github.com/romshark/toki/internal/tokibundle"
)

// UnknownCommandError is the error of a command that toki doesn't have.
// It is [ErrUnknownCommand].
type UnknownCommandError struct{ Command string }

func (e *UnknownCommandError) Error() string {
	return fmt.Sprintf("%v %q, run 'toki help' for a list of commands",
		ErrUnknownCommand, e.Command)
}

func (*UnknownCommandError) Is(target error) bool { return target == ErrUnknownCommand }

// BundleCorruptError is the error of a bundle with native-locale messages
// that disagree with their TIK. It is [ErrBundleCorrupt].
type BundleCorruptError struct{ Corrupt int64 }

func (e *BundleCorruptError) Error() string {
	return fmt.Sprintf("%v (%d corrupt native-locale entries)", ErrBundleCorrupt, e.Corrupt)
}

func (*BundleCorruptError) Is(target error) bool { return target == ErrBundleCorrupt }

// LocaleMismatchError is the error of a flag -l that differs from the
// default locale of the bundle.
type LocaleMismatchError struct {
	Flag, Bundle string // The locales of the flag and of the bundle.
	BundlePkg    string // The path of the bundle package.
}

func (e *LocaleMismatchError) Error() string {
	return fmt.Sprintf("parameter -l (%q) must either match "+
		"DefaultLocale (%q) in package %q or not be set at all",
		e.Flag, e.Bundle, e.BundlePkg)
}

// errorText describes err in the language of tr. It translates the errors
// of the command line, and leaves the details that the errors of other
// layers add in English.
func errorText(tr tokibundle.Reader, err error) string {
	var (
		unknownCommand *UnknownCommandError
		bundleCorrupt  *BundleCorruptError
		localeMismatch *LocaleMismatchError
		localeFlag     *config.LocaleFlagError
	)
	if detail, ok := wrapped(err, ErrInvalidCLIArgs); ok {
		// The placeholder is the error of the arguments, in English unless
		// it is one of the errors this domain translates.
		return tr.String("invalid arguments: {text}", errorText(tr, detail))
	}
	if detail, ok := wrapped(err, ErrAnalyzingSource); ok {
		// The placeholder is the error of the analysis in English.
		return tr.String("analyzing sources: {text}", detail.Error())
	}
	switch {
	case errors.As(err, &unknownCommand):
		// The placeholder is the command that toki doesn't have. Keep toki
		// help, a command.
		return tr.String("unknown command \"{text}\", run 'toki help' for a list of commands",
			unknownCommand.Command)
	case errors.As(err, &bundleCorrupt):
		// Keep toki repair, a command.
		return tr.String("bundle contains corrupt native-locale messages — run `toki repair` "+
			"({# corrupt native-locale entries})", bundleCorrupt.Corrupt)
	case errors.As(err, &localeMismatch):
		// Keep -l, a flag, and DefaultLocale, a Go constant. The placeholders
		// are the locale of the flag, the default locale of the bundle and the
		// path of the bundle package.
		return tr.String("parameter -l (\"{text}\") must either match "+
			"DefaultLocale (\"{text}\") in package \"{text}\" or not be set at all",
			localeMismatch.Flag, localeMismatch.Bundle, localeMismatch.BundlePkg)
	case errors.As(err, &localeFlag):
		if localeFlag.Err == nil {
			// The placeholders are a flag, as l, and its value. Keep BCP 47 and
			// "und", the undetermined locale.
			return tr.String("argument {text}=\"{text}\": "+
				"must be a valid non-und BCP 47 locale: is und",
				localeFlag.Flag, localeFlag.Value)
		}
		// The placeholders are a flag, as l, its value and the error of
		// parsing it in English. Keep BCP 47 and "und", the undetermined
		// locale.
		return tr.String("argument {text}=\"{text}\": "+
			"must be a valid non-und BCP 47 locale: {text}",
			localeFlag.Flag, localeFlag.Value, localeFlag.Err.Error())
	case errors.Is(err, ErrMissingLocaleParam):
		// Keep BCP 47, "und", the undetermined locale, and 'l', a flag.
		return tr.String("please provide a valid non-und BCP 47 locale for the default " +
			"language of your original code base using the 'l' parameter")
	case errors.Is(err, ErrSourceErrors):
		return tr.String("source code contains errors")
	case errors.Is(err, ErrBundleIncomplete):
		return tr.String("bundle contains incomplete catalogs")
	case errors.Is(err, ErrDesktopUnavailable):
		// Keep cgo, the Go feature, and toki edit -server <addr>, a command.
		return tr.String("this build of toki was compiled without cgo; " +
			"the native desktop app is unavailable — " +
			"run `toki edit -server <addr>` instead")
	}
	return err.Error()
}

// wrapped returns the error that err, an error from
// fmt.Errorf("%w: %w", sentinel, other), wraps besides sentinel, and false
// when err isn't one.
func wrapped(err, sentinel error) (error, bool) {
	u, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return nil, false
	}
	if errs := u.Unwrap(); len(errs) == 2 && errs[0] == sentinel {
		return errs[1], true
	}
	return nil, false
}
