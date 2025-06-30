package tik_test

import (
	"testing"

	tikgo "github.com/romshark/tik/tik-go"

	"github.com/romshark/toki/internal/tik"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestProducesCompleteICU(t *testing.T) {
	p := tikgo.NewParser(tikgo.DefaultConfig)
	f := func(t *testing.T, expect bool, locale language.Tag, input string) {
		t.Helper()
		err := p.ParseFn(input, func(tk tikgo.TIK) {
			require.Equal(t, expect, tik.ProducesCompleteICU(locale, tk))
		})
		require.NoError(t, err.Err)
	}

	f(t, true, language.English, "This produces a complete ICU message")
	f(t, true, language.Ukrainian, "Це створює повне повідомлення")
	f(t, true, language.Korean, "이렇게 하면 완전한 메시지가 생성됩니다.")
	f(t, true, language.Japanese, "これにより完全なICUメッセージが生成される")
	f(t, true, language.English, `
		Neither of these placeholders require explicit translation:
		{text} {number} {integer} {date-full} {date-long} {date-medium} {date-short}
		{time-full} {time-long} {time-medium} {time-short} {currency}
	`)
	f(t, true, language.Ukrainian, `
		Жоден із цих заповнювачів не потребує явного перекладу:
		{text} {number} {integer} {date-full} {date-long} {date-medium} {date-short}
		{time-full} {time-long} {time-medium} {time-short} {currency}
	`)
	f(t, true, language.Japanese, `
		これらのプレースホルダーはどちらも明示的な翻訳を必要としません:
		{text} {number} {integer} {date-full} {date-long} {date-medium} {date-short}
		{time-full} {time-long} {time-medium} {time-short} {currency}
	`)
	f(t, true, language.Korean, `
		다음 플레이스홀더 중 어느 것도 명시적인 번역이 필요하지 않습니다.
		{text} {number} {integer} {date-full} {date-long} {date-medium} {date-short}
		{time-full} {time-long} {time-medium} {time-short} {currency}
	`)

	// Cardinal plural.
	f(t, true, language.Japanese, "{name} - {# 件のメッセージ}")
	f(t, true, language.Korean, "{name} - {# 개의 메시지}")
	f(t, false, language.English, "{name} - {# messages}")
	f(t, false, language.German, "{name} - {# Nachrichten}")
	f(t, false, language.Ukrainian, "{name} - {# повідомлення}")

	// Ordinal plural.
	f(t, true, language.Japanese, "{name}は{ordinal}だ")
	f(t, true, language.Korean, "{name}은(는) {ordinal}이다.")
	f(t, false, language.English, "{name} is {ordinal}")
	f(t, true, language.German, "{name} ist {ordinal}")
	f(t, false, language.Ukrainian, "{name} - {ordinal}")
}
