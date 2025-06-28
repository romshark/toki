package icu_test

import (
	"testing"

	"github.com/romshark/toki/internal/codeparse"
	"github.com/romshark/toki/internal/icu"

	"github.com/romshark/icumsg"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestAnalysisReport(t *testing.T) {
	tk := new(icumsg.Tokenizer)
	var buffer []icumsg.Token
	f := func(t *testing.T, locale language.Tag, input string, expectReport []string) {
		t.Helper()
		var err error
		buffer = buffer[:0]
		buffer, err = tk.Tokenize(locale, buffer, input)
		require.NoError(t, err)
		report := icu.AnalysisReport(
			locale, input, buffer, codeparse.ICUSelectOptions,
		)
		require.Equal(t, expectReport, report)
	}

	// Complete.
	f(t, language.English, "complete message", nil)
	f(t, language.BritishEnglish, "complete message", nil)
	f(t, language.AmericanEnglish, "complete message", nil)
	f(t, language.German, "vollwertige Nachricht", nil)
	f(t, language.Russian, "полноценное сообщение", nil)
	f(t, language.Ukrainian, "повноцінне повідомлення", nil)

	// Complete - Cardinal.
	f(t, language.English,
		"{var0, plural, other {# things} one {# thing}}",
		nil)
	f(t, language.BritishEnglish,
		"{var0, plural, other {# things} one {# thing}}",
		nil)
	f(t, language.German,
		`{var0, plural,
			one {# unvollständige Nachricht}
			other {# unvollständige Nachrichten}
		}`,
		nil)
	f(t, language.Russian,
		`{var0, plural,
			one {# незавершённое собщение}
			few {# незавершённых собщений}
			many {# незавершённых собщений}
			other {# незавершённых собщений}
		}`,
		nil)
	f(t, language.MustParse("cy"),
		`{var0, plural,
			zero {# cŵn}
			one {# ci}
			two {# gi}
			few {# chi}
			many {# chi}
			other {# ci}
		}`,
		nil)

	// Complete - Ordinal.
	f(t, language.English,
		"{var0, selectordinal, other {#th} one {#st} few {#rd} two {#nd}}",
		nil)
	f(t, language.BritishEnglish,
		"{var0, selectordinal, other {#th} one {#st} few {#rd} two {#nd}}",
		nil)
	f(t, language.German,
		`{var0, selectordinal, other {#.}}`,
		nil)
	f(t, language.Russian,
		"{var0, selectordinal, other {#-й}}",
		nil)
	f(t, language.Ukrainian,
		"{var0, selectordinal, few {#-я} other {#-а}}",
		nil)
	f(t, language.MustParse("cy"),
		`{var0, selectordinal,
			zero {yr 0fed}
			one {yr 1af}
			two {yr 2il}
			few {yr 3ydd}
			many {yr 20fed}
			other {yr #ain}
		}`,
		nil)

	// Complete - Gender.
	f(t, language.English,
		`{
			var0_gender, select,
			other {{var0} notified}
			female {{var0} notified}
			male {{var0} notified}
		}`,
		nil)
	f(t, language.Russian,
		`{
			var0_gender, select,
			other {{var0} сообщил}
			female {{var0} сообщила}
			male {{var0} сообщил}
		}`,
		nil)
	f(t, language.Ukrainian,
		`{
			var0_gender, select,
			female {{var0} повідомила}
			male {{var0} повідомив}
			other {{var0} повідомило}
		}`,
		nil)

	// Incomplete - Cardinal.
	f(t, language.English,
		"{var0, plural, other {# things}}",
		[]string{`Argument "var0" is missing options [one]`})
	f(t, language.German,
		"{var0, plural, other {# Dinge}}",
		[]string{`Argument "var0" is missing options [one]`})

	f(t, language.Russian,
		"{var0, plural, other {# незавершённых собщений}}",
		[]string{`Argument "var0" is missing options [one,few,many]`})
	f(t, language.Russian,
		`{
			var0, plural,
			other {# незавершённых собщений}
			one {# незавершённое собщение}
		}`,
		[]string{`Argument "var0" is missing options [few,many]`})
	f(t, language.Russian,
		`{var0, plural,
			=0 {нет незавершённых собщений}
			other {# незавершённых собщений}
			few {# незавершённых собщений}
			one {# незавершённое собщение}
		}`,
		[]string{`Argument "var0" is missing options [many]`})
	f(t, language.Latvian,
		`{var0, plural, other {# diennaktis}}`,
		[]string{`Argument "var0" is missing options [zero,one]`})

	// Incomplete - Ordinal.
	f(t, language.English,
		"{var0, selectordinal, other {#th}}",
		[]string{`Argument "var0" is missing options [one,two,few]`})
	f(t, language.BritishEnglish,
		"{var0, selectordinal, other {#th} one {#st}}",
		[]string{`Argument "var0" is missing options [two,few]`})
	f(t, language.AmericanEnglish,
		"{var0, selectordinal, other {#th} one {#st} few {#rd}}",
		[]string{`Argument "var0" is missing options [two]`})
	f(t, language.Ukrainian,
		"{var0, selectordinal, other {#-а}}",
		[]string{`Argument "var0" is missing options [few]`})
	f(t, language.MustParse("cy"),
		`{var0, selectordinal,
			zero {yr 0fed}
			one {yr 1af}
			two {yr 2il}
			few {yr 3ydd}
			other {yr #ain}
		}`,
		[]string{`Argument "var0" is missing options [many]`})
	f(t, language.MustParse("cy"),
		`{var0, selectordinal,
			zero {yr 0fed}
			one {yr 1af}
			few {yr 3ydd}
			many {yr 20fed}
			other {yr #ain}
		}`,
		[]string{`Argument "var0" is missing options [two]`})

	// Incomplete - Gender.
	f(t, language.English,
		"{var0_gender, select, other {{var0}} female {{var0}}} notified",
		[]string{`Argument "var0_gender" is missing options [male]`})
	f(t, language.Russian,
		"{var0_gender, select, other {{var0} сообщил}}",
		[]string{`Argument "var0_gender" is missing options [male,female]`})
	f(t, language.Ukrainian,
		"{var0_gender, select, male {{var0} повідомив} other {{var0} повідомило}}",
		[]string{`Argument "var0_gender" is missing options [female]`})

	// Multiple incomplete arguments.
	f(t, language.Ukrainian,
		`{
			var0_gender, select,
			other {{var0}}
		} and {
			var1_gender, select,
			female {{var1}}
			other {{var1}}
		} and {
			var2_gender, select,
			male {{var1}}
			other {{var1}}
		}`,
		[]string{
			`Argument "var0_gender" is missing options [male,female]`,
			`Argument "var1_gender" is missing options [male]`,
			`Argument "var2_gender" is missing options [female]`,
		})

	// Multiple nested incomplete arguments.
	f(t, language.Ukrainian,
		`{
			var0_gender, select,
			other {
				{var1_gender, select,
					other {
						{var0} {var1}		
					}
				}
			}
		}`,
		[]string{
			`Argument "var1_gender" is missing options [male,female]`,
			`Argument "var0_gender" is missing options [male,female]`,
		})

	f(t, language.Ukrainian,
		`{
			var0_gender, select,
			other {
				{var1, selectordinal,
					other{
						{var0} перше отримало {var2, plural,
							other {# повідомлень}
						}
					}
				}
			}
		}`,
		[]string{
			`Argument "var2" is missing options [one,few,many]`,
			`Argument "var1" is missing options [few]`,
			`Argument "var0_gender" is missing options [male,female]`,
		})
}
