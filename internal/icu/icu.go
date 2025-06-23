package icu

import (
	"fmt"
	"iter"
	"strings"

	"github.com/romshark/icumsg"
	"golang.org/x/text/language"
)

func AnalysisReport(
	locale language.Tag, raw string, tokens []icumsg.Token,
	selectOptions icumsg.SelectOptions,
) (report []string) {
	errsSeq := icumsg.Errors(locale, raw, tokens, selectOptions)
	for err := range errsSeq {
		switch err := err.(type) {
		case icumsg.ErrorPluralMissingOption:
			argName := tokens[err.TokenIndex+1].String(raw, tokens)
			missingOptions := missingOptions(err)
			report = append(report, fmt.Sprintf("Argument %q is missing options %s",
				argName, missingOptions))
		case icumsg.ErrorSelectMissingOption:
			argName := tokens[err.TokenIndex+1].String(raw, tokens)
			missingOptions := missingOptions(err)
			report = append(report, fmt.Sprintf("Argument %q is missing options %s",
				argName, missingOptions))
		case icumsg.ErrorSelectInvalidOption:
			argName := tokens[err.TokenIndexArgument+1].String(raw, tokens)
			optionName := tokens[err.TokenIndexOption].String(raw, tokens)
			report = append(report, fmt.Sprintf("Argument %q: invalid select option %q",
				argName, optionName))
		}
	}
	return report
}

func missingOptions[E interface{ MissingOptions() iter.Seq[string] }](e E) string {
	var b strings.Builder
	sep := ""
	b.WriteByte('[')
	for o := range e.MissingOptions() {
		b.WriteString(sep)
		b.WriteString(o)
		sep = ","
	}
	b.WriteByte(']')
	return b.String()
}
