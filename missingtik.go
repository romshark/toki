package main

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/language"

	"github.com/romshark/toki/internal/tokibundle"
)

// missingTranslationFunc is the type of [tokibundle.MissingTranslation].
type missingTranslationFunc = func(
	w io.Writer, locale language.Tag, tik string, args ...any,
) (int, error)

// writeMissingTIK returns the [tokibundle.MissingTranslation] of toki. It
// leaves a catalog that lacks the message of a TIK to fallback, the default,
// which falls back to the catalog of the default locale. Where that lacks it
// too, which fallback answers with a panic, it writes the TIK itself, with
// its arguments in place of its placeholders. That happens to a TIK that
// toki generate hasn't added to the catalogs yet. As toki generates its own
// bundle, the panic would keep it from running toki generate.
func writeMissingTIK(fallback missingTranslationFunc) missingTranslationFunc {
	return func(w io.Writer, locale language.Tag, tik string, args ...any) (int, error) {
		if locale != tokibundle.Default().Locale() {
			return fallback(w, locale, tik, args...)
		}
		return io.WriteString(w, renderTIK(tik, args))
	}
}

// renderTIK renders tik without a catalog: without its context, with its
// escapes resolved and with args in place of its placeholders, in order.
// A plural shows its number in place of #, as "3 files" for {# files}.
func renderTIK(tik string, args []any) string {
	if strings.HasPrefix(tik, "[") {
		if _, after, ok := strings.Cut(tik, "] "); ok {
			tik = after
		}
	}
	var b strings.Builder
	next := func() {
		if len(args) == 0 {
			return
		}
		if s, ok := args[0].(tokibundle.String); ok {
			b.WriteString(s.Value)
		} else {
			_, _ = fmt.Fprint(&b, args[0])
		}
		args = args[1:]
	}
	for i := 0; i < len(tik); i++ {
		switch c := tik[i]; c {
		case '\\': // An escape: the next character as it is.
			if i+1 < len(tik) {
				i++
				b.WriteByte(tik[i])
			}
		case '{':
			if strings.HasPrefix(tik[i+1:], "#") {
				// A plural, whose text follows its number until its }.
				next()
				i++
				continue
			}
			next()
			if end := strings.IndexByte(tik[i:], '}'); end >= 0 {
				i += end
			}
		case '}': // The end of a plural.
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
