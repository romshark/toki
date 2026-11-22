package gengo

import (
	"strings"
	"testing"

	"github.com/romshark/icumsg"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestUnescapeICULiteral(t *testing.T) {
	f := func(t *testing.T, raw, expect string) {
		t.Helper()
		requireSingleLiteral(t, raw, raw)
		require.Equal(t, expect, unescapeICULiteral(raw))
	}

	f(t, "plain text", "plain text")
	f(t, "He's here", "He's here")
	f(t, "It''s fine", "It's fine")
	f(t, "''", "'")
	f(t, "'leading", "'leading")
	f(t, "trailing '", "trailing '")
	f(t, "'{'braces'}'", "{braces}")
	f(t, "'{it''s quoted}'", "{it's quoted}")
	f(t, "'{' isn't '}'", "{ isn't }")
	f(t, "Use '#' here", "Use '#' here") // '#' is syntax only in plural options.
}

func TestIterLiteralPartsPlural(t *testing.T) {
	f := func(t *testing.T, raw, expect string) {
		t.Helper()
		requireSingleLiteral(t, "{var0, plural, other {"+raw+"}}", raw)
		var b strings.Builder
		for s, numberSign := range iterLiteralParts(raw, true) {
			if numberSign {
				b.WriteString("<N>")
				continue
			}
			b.WriteString(s)
		}
		require.Equal(t, expect, b.String())
	}

	f(t, "files", "files")
	f(t, "#", "<N>")
	f(t, "# files", "<N> files")
	f(t, "## files", "<N><N> files")
	f(t, "it's # files", "it's <N> files")
	f(t, "it''s #", "it's <N>")
	f(t, "''#", "'<N>")
	f(t, "'#'1 and #", "#1 and <N>")
	f(t, "'''#'", "'#")
	f(t, "'#''s'", "#'s")
	f(t, "'#' is '{'#'}'", "# is {<N>}")
}

// requireSingleLiteral requires icumsg to tokenize raw in msg as a single literal.
func requireSingleLiteral(t *testing.T, msg, raw string) {
	t.Helper()
	var tokenizer icumsg.Tokenizer
	tokens, err := tokenizer.Tokenize(language.English, nil, msg)
	require.NoError(t, err)
	var literals []string
	for _, tok := range tokens {
		if tok.Type == icumsg.TokenTypeLiteral {
			literals = append(literals, tok.String(msg, tokens))
		}
	}
	require.Equal(t, []string{raw}, literals)
}
