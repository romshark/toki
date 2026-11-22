package template

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/romshark/icumsg"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/romshark/toki/internal/icu"
	"github.com/romshark/toki/internal/tokibundle"
)

// fmtInt formats n for the language of tr, as 10,000 in English.
func fmtInt(tr tokibundle.Reader, n int) string {
	return tr.Translator().FmtNumber(float64(n), 0)
}

// fmtPercent formats ratio, from 0 to 1, as a percentage for the language of
// tr, with up to 2 decimals and no trailing zeros, as 12.5% in English. A
// positive ratio too small for 2 decimals shows as <0.01%.
func fmtPercent(tr tokibundle.Reader, ratio float64) string {
	pct := ratio * 100
	if pct > 0 && pct < 0.01 {
		return "<" + tr.Translator().FmtPercent(0.01, 2)
	}
	s := strings.TrimRight(strconv.FormatFloat(pct, 'f', 2, 64), "0")
	_, decimals, _ := strings.Cut(s, ".")
	return tr.Translator().FmtPercent(pct, uint64(len(decimals)))
}

// FmtDuration formats d for the language of tr.
func FmtDuration(tr tokibundle.Reader, d time.Duration) string {
	if d < time.Second {
		// A duration in milliseconds, as 250 ms.
		return tr.String("{text} ms", tr.Translator().FmtNumber(float64(d.Milliseconds()), 0))
	}
	// A duration in seconds, as 1.5 s.
	return tr.String("{text} s", tr.Translator().FmtNumber(d.Seconds(), 1))
}

// localeName names locale in the language of tr, as German for de in
// English. It returns "" when it knows no name.
func localeName(tr tokibundle.Reader, locale string) string {
	tag, err := language.Parse(locale)
	if err != nil {
		return ""
	}
	namer := display.Languages(tr.Locale())
	if namer == nil {
		return ""
	}
	return capitalize(namer.Name(tag))
}

// ownLangName names language t in t itself, as Deutsch for de, which is how a
// language picker lists it.
func ownLangName(t language.Tag) string {
	return capitalize(display.Self.Name(t))
}

// capitalize returns s with its first letter in upper case, as languages
// such as Russian name languages in lower case.
func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// notFoundText is the text of [PageNotFound].
func notFoundText(tr tokibundle.Reader) string {
	// The page for a URL that doesn't exist shows this text as if it were an
	// ICU message. The placeholders show {name} and {text} as they are, as
	// placeholders that nothing fills. Keep the line breaks.
	return tr.String("Hello, {text}!\n"+
		"The page \"{text}\" you asked for doesn't exist.\n"+
		"Want to go back, or to the dashboard?", "{name}", "{text}")
}

// ICUError is a syntax error in an ICU message.
type ICUError struct {
	// Index is the byte index in the message at which parsing failed.
	Index int
	// Err is the error of the parser, one of the errors of package icumsg.
	Err error
}

// icuErrorText describes e in the language of tr.
func icuErrorText(tr tokibundle.Reader, e *ICUError) string {
	switch {
	case errors.Is(e.Err, icumsg.ErrUnclosedQuote):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Unclosed quote at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrUnexpectedToken):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Unexpected token at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrUnexpectedEOF):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Unexpected end of the message at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrExpectedComma):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Expected a comma at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrExpectedColon):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Expected a colon at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrExpectBracketOpen):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Expected an opening brace at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrExpectBracketClose):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Expected a closing brace at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrMissingOptionOther):
		// ICU syntax error. Keep "other", the name of an ICU option. The
		// placeholder is the byte index of the error.
		return tr.String("Missing the mandatory option other at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrInvalidOption):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Invalid option at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrDuplicateOption):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Duplicate option at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrInvalidArgNumber):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Invalid argument number at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrNestingTooDeep):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Nesting too deep at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrInvalidOffset):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("Invalid offset at index {integer}.", e.Index)
	case errors.Is(e.Err, icumsg.ErrUnsupportedPluralRule):
		// ICU syntax error. The placeholder is the byte index of the error.
		return tr.String("The locale doesn't support this plural rule at index {integer}.", e.Index)
	}
	// ICU syntax error that the editor has no text for. The first placeholder
	// is the byte index of the error, the second the error in English.
	return tr.String("Error at index {integer}: {text}", e.Index, e.Err.Error())
}

// problemText describes problem p of an ICU message in the language of tr.
func problemText(tr tokibundle.Reader, p icu.Problem) string {
	if p.Invalid != "" {
		// Note under an ICU message editor. The first placeholder is the name of
		// an ICU argument, the second the name of a select option.
		return tr.String("Argument {text} has the invalid select option {text}.", p.Arg, p.Invalid)
	}
	// Note under an ICU message editor. The first placeholder is the name of
	// an ICU argument, the last lists the names of the plural or select options
	// it lacks, as "one, few".
	return tr.String("Argument {text} is missing {# options}: {text}.",
		p.Arg, len(p.Missing), strings.Join(p.Missing, ", "))
}

// ProjectErrorKind is the reason the editor can't load a project.
type ProjectErrorKind uint8

const (
	// ProjectErrorNone means the project loaded.
	ProjectErrorNone ProjectErrorKind = iota
	// ProjectErrorNoDir means no project directory is selected.
	ProjectErrorNoDir
	// ProjectErrorBundleMissing means the bundle file Path doesn't exist.
	ProjectErrorBundleMissing
	// ProjectErrorBundleCheck means checking the bundle file Path failed
	// with Err.
	ProjectErrorBundleCheck
	// ProjectErrorAnalyze means analyzing the source code failed with Err.
	ProjectErrorAnalyze
	// ProjectErrorSource means the source code has errors, which the
	// source errors of the project list.
	ProjectErrorSource
	// ProjectErrorLoadEdits means loading the unsaved edits failed with Err.
	ProjectErrorLoadEdits
	// ProjectErrorIndex means populating the index DB failed with Err.
	ProjectErrorIndex
)

// ProjectError is why the editor can't load a project.
type ProjectError struct {
	Kind ProjectErrorKind
	Path string // The bundle file, see Kind.
	Err  string // The message of the underlying error, see Kind.
}

// Error describes e in English for the log.
func (e ProjectError) Error() string {
	switch e.Kind {
	case ProjectErrorNoDir:
		return "no directory selected"
	case ProjectErrorBundleMissing:
		return fmt.Sprintf("bundle file %q not found", e.Path)
	case ProjectErrorBundleCheck:
		return fmt.Sprintf("checking bundle file %q: %s", e.Path, e.Err)
	case ProjectErrorAnalyze:
		return "analyzing source: " + e.Err
	case ProjectErrorSource:
		return "source code contains errors"
	case ProjectErrorLoadEdits:
		return "loading unsaved edits: " + e.Err
	case ProjectErrorIndex:
		return "populating index DB: " + e.Err
	}
	return ""
}

// projectErrorText describes e in the language of tr.
func projectErrorText(tr tokibundle.Reader, e ProjectError) string {
	switch e.Kind {
	case ProjectErrorNoDir:
		return tr.String("No project folder selected.")
	case ProjectErrorBundleMissing:
		// The placeholder is the path of a file. Keep toki generate, a command.
		return tr.String("Bundle file {text} not found. Run toki generate first.", e.Path)
	case ProjectErrorBundleCheck:
		// The first placeholder is the path of a file, the second an error in
		// English.
		return tr.String("Checking bundle file {text} failed: {text}", e.Path, e.Err)
	case ProjectErrorAnalyze:
		// The placeholder is an error in English.
		return tr.String("Analyzing the source code failed: {text}", e.Err)
	case ProjectErrorSource:
		return tr.String("The source code contains errors.")
	case ProjectErrorLoadEdits:
		// The placeholder is an error in English.
		return tr.String("Loading the unsaved changes failed: {text}", e.Err)
	case ProjectErrorIndex:
		// The placeholder is an error in English. The index database is
		// where the editor caches the scan of the project.
		return tr.String("Populating the index database failed: {text}", e.Err)
	}
	return ""
}
