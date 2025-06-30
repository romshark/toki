package tik

import (
	"github.com/romshark/icumsg/cldr"
	tikgo "github.com/romshark/tik/tik-go"
	"golang.org/x/text/language"
)

// ProducesCompleteICU returns true for TIKs that produce a complete ICU message
// that doesn't require no further translation. For example, a TIK like
// `Hello World` generates a complete ICU message for all locales, whereas the TIK
// `You have {# unread messages}` only produces a complete ICU for languages like
// Japanese, Korean and others that only support the "other" cardinal plural rule.
// For English, the TIK `You have {# unread messages}` would require an addiotional
// "one" rule that can't be produced from the TIK.
// Similarly, TIKs containing the `{ordinal}` placeholder like
// `You're {ordinal} in the waiting queue` can't produce a complete ICU message for
// English, which requires additional "one", "two" and "few" ordinal plural rules,
// whereas for Japanese it produces a complete one because it only requires rule "other".
// Other placeholders like `Today is {date-full}` always produce complete ICU messages.
func ProducesCompleteICU(locale language.Tag, t tikgo.TIK) bool {
	cardReq, ordReq := cldr.LocalePluralRules(locale)
	onlyOther := cldr.PluralRules{Other: true}
	cardOtherOnly, ordOtherOnly := cardReq == onlyOther, ordReq == onlyOther
	for _, t := range t.Tokens {
		switch t.Type {
		case tikgo.TokenTypeCardinalPluralStart:
			if !cardOtherOnly {
				return false
			}
		case tikgo.TokenTypeOrdinalPlural:
			if !ordOtherOnly {
				return false
			}
		}
	}
	return true
}
