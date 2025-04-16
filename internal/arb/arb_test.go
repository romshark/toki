package arb_test

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/romshark/icumsg"
	"github.com/romshark/toki/internal/arb"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestDecode(t *testing.T) {
	t.Parallel()
	arbDecoder := arb.NewDecoder()

	f := func(t *testing.T, expect arb.File, input string) {
		t.Helper()
		f, err := arbDecoder.Decode(strings.NewReader(input))
		require.NoError(t, err)
		require.Equal(t, expect.Author, f.Author)
		require.Equal(t, expect.Comment, f.Comment)
		require.Equal(t, expect.Locale, f.Locale)
		require.Equal(t, expect.Context, f.Context)
		require.Equal(t, expect.LastModified, f.LastModified)
		require.Equal(t, expect.CustomAttributes, f.CustomAttributes)
		require.Len(t, f.Messages, len(f.Messages))
		for k, msg := range f.Messages {
			expect := expect.Messages[k]
			require.Equal(t, expect.ID, msg.ID)
			require.Equal(t, expect.Context, msg.Context)
			require.Equal(t, expect.Description, msg.Description)
			require.Equal(t, expect.Comment, msg.Comment)
			require.Equal(t, expect.Placeholders, msg.Placeholders)
			require.Equal(t, expect.ICUMessage, msg.ICUMessage)
			require.Equal(t, expect.Type, msg.Type)
			require.NotEmpty(t, msg.ICUMessageTokens)
		}

		var b bytes.Buffer
		err = arb.Encode(&b, f, "    ")
		require.NoError(t, err)
		require.Equal(t, input, b.String())
	}

	f(t, arb.File{
		Locale:           language.Ukrainian,
		Context:          "HomePage",
		CustomAttributes: map[string]any{},
		Messages: map[string]arb.Message{
			"helloAndWelcome": {
				ID:          "helloAndWelcome",
				ICUMessage:  "Ласкаво просимо, {firstName} {lastName}!",
				Type:        arb.MessageTypeText,
				Description: "Initial welcome message",
				Placeholders: map[string]arb.Placeholder{
					"firstName": {
						Type: arb.PlaceholderString,
					},
					"lastName": {
						Type: arb.PlaceholderString,
					},
				},
			},
			"newMessages": {
				ID: "newMessages",
				ICUMessage: "У вас {newMessages, plural, " +
					"one {# нове повідомлення} " +
					"few {# нових повідомлення} " +
					"many {# нових повідомлень} " +
					"other {# нових повідомлень}}",
				Type:        arb.MessageTypeText,
				Description: "Number of new messages in inbox.",
				Placeholders: map[string]arb.Placeholder{
					"newMessages": {
						Type: arb.PlaceholderInt,
					},
				},
				Context: "Test Context",
				Comment: "Test comment",
			},
		},
	}, MustReadFile(t, "testdata/simple.arb"))

	f(t, arb.File{
		Locale:           MustParseLocale(t, "de-CH"),
		CustomAttributes: map[string]any{},
		Messages:         map[string]arb.Message{},
	}, MustReadFile(t, "testdata/barebones.arb"))

	f(t, arb.File{
		Locale: language.English,
		CustomAttributes: map[string]any{
			"@@x-generator":      "Foo",
			"@@x-something-else": "Bar Bazz",
		},
		LastModified: time.Date(2025, 4, 12, 20, 0o3, 44, 0, time.UTC),
		Messages: map[string]arb.Message{
			"msgWithCustomAttr": {
				ID:         "msgWithCustomAttr",
				Type:       "text",
				ICUMessage: "Translation",
				Description: "This message has custom attributes " +
					"x-src and x-something-else",
				CustomAttributes: map[string]any{
					"x-src":            "foo/bar/main.go:14",
					"x-something-else": "bazz",
				},
			},
		},
	}, MustReadFile(t, "testdata/custom_attributes.arb"))
}

func TestDecodeDefaultMessageType(t *testing.T) {
	t.Parallel()
	arbDecoder := arb.NewDecoder()

	fc := MustReadFile(t, "testdata/msg_type_fallback.arb")

	file, err := arbDecoder.Decode(strings.NewReader(fc))
	require.NoError(t, err)

	expect := arb.File{
		Locale:           language.English,
		CustomAttributes: map[string]any{},
		Messages: map[string]arb.Message{
			"x": {
				ID:               "x",
				ICUMessage:       "Simple message",
				Type:             arb.MessageTypeText,
				Description:      "The message type is intentionally undefined",
				CustomAttributes: map[string]any{},
				ICUMessageTokens: []icumsg.Token{
					{
						Type:       icumsg.TokenTypeLiteral,
						IndexStart: 0,
						IndexEnd:   len("Simple message"),
					},
				},
			},
		},
	}

	require.Equal(t, expect, *file)

	var out bytes.Buffer

	err = arb.Encode(&out, file, "    ")
	require.NoError(t, err)

	expectEncoded := MustReadFile(t, "testdata/msg_type_fallback_expect.arb")

	require.Equal(t, expectEncoded, out.String())
}

func TestDecodeErr(t *testing.T) {
	t.Parallel()
	arbDecoder := arb.NewDecoder()

	f := func(t *testing.T, expectErr error, expectErrMsg, input string) {
		t.Helper()
		f, err := arbDecoder.Decode(strings.NewReader(input))
		require.ErrorIs(t, err, expectErr)
		require.Zero(t, f)
		require.EqualError(t, err, expectErrMsg)
	}

	f(t, arb.ErrEmptyICUMsg,
		`for key "invalidMsg": empty ICU message`,
		MustReadFile(t, "testdata/err_empty_icu_msg.arb"))
	f(t, arb.ErrInvalidICUMessage,
		"invalid ICU message: at index 9: missing the mandatory 'other' option",
		MustReadFile(t, "testdata/err_invalid_icu_msg.arb.txt"))
	f(t, arb.ErrMissingRequiredLocale,
		`missing required @@locale`,
		MustReadFile(t, "testdata/err_missing_locale.arb.txt"))
	f(t, arb.ErrInvalid,
		`invalid @@locale value "invalid": language: tag is not well-formed`,
		MustReadFile(t, "testdata/err_invalid_locale.arb"))
	f(t, arb.ErrMalformedJSON,
		"malformed JSON: invalid character '}' "+
			"looking for beginning of object key string",
		MustReadFile(t, "testdata/err_malformed.arb.txt"))
	f(t, arb.ErrMalformedJSON,
		"malformed JSON: invalid character '}' "+
			"looking for beginning of object key string",
		MustReadFile(t, "testdata/err_malformed_meta.arb.txt"))
	f(t, arb.ErrInvalid,
		`invalid message type: unsupported message type: "invalid"`,
		MustReadFile(t, "testdata/err_invalid_msg_type.arb.txt"))
	f(t, arb.ErrInvalid,
		`invalid (for key "placeholder"): unsupported placeholder type: "invalid"`,
		MustReadFile(t, "testdata/err_invalid_placeholder_type.arb.txt"))
	f(t, arb.ErrInvalid,
		`invalid @@last_modified format: `+
			`parsing time "15:40" as "2006-01-02T15:04:05Z07:00": `+
			`cannot parse "15:40" as "2006"`,
		MustReadFile(t, "testdata/err_invalid_last_modified.arb"))
	f(t, arb.ErrUndefinedPlaceholder,
		`undefined placeholder: "notInList"`,
		MustReadFile(t, "testdata/err_undefined_placeholder.arb.txt"))
}

func MustReadFile(tb testing.TB, fileName string) string {
	tb.Helper()
	c, err := os.ReadFile(fileName)
	require.NoError(tb, err)
	return string(c)
}

func MustParseLocale(tb testing.TB, str string) language.Tag {
	tb.Helper()
	tag, err := language.Parse(str)
	require.NoError(tb, err)
	return tag
}

func BenchmarkDecode(b *testing.B) {
	arbDecoder := arb.NewDecoder()
	var f *arb.File
	var err error

	fc := MustReadFile(b, "testdata/large.arb")
	r := strings.NewReader(fc)
	for b.Loop() {
		r.Reset(fc)
		if f, err = arbDecoder.Decode(r); err != nil {
			panic(err)
		}
	}
	runtime.KeepAlive(f)
	runtime.KeepAlive(err)
}

func BenchmarkEncode(b *testing.B) {
	arbDecoder := arb.NewDecoder()
	fc := MustReadFile(b, "testdata/large.arb")
	f, err := arbDecoder.Decode(strings.NewReader(fc))
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	for b.Loop() {
		buf.Reset()
		if err := arb.Encode(&buf, f, "   "); err != nil {
			panic(err)
		}
	}
}
