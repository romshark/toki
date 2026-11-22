package icu

import (
	"fmt"
	"slices"
	"strings"

	"github.com/romshark/icumsg"
	"golang.org/x/text/language"
)

// Problem is a plural or select argument of an ICU message that lacks options
// its locale needs, or that has a select option that isn't allowed.
type Problem struct {
	// Arg is the name of the argument.
	Arg string
	// Missing lists the options the argument lacks. It's empty when Invalid is set.
	Missing []string
	// Invalid is the select option that isn't allowed.
	Invalid string
}

// String describes p in English.
func (p Problem) String() string {
	if p.Invalid != "" {
		return fmt.Sprintf("Argument %q: invalid select option %q", p.Arg, p.Invalid)
	}
	return fmt.Sprintf("Argument %q is missing options [%s]",
		p.Arg, strings.Join(p.Missing, ","))
}

// Analyze returns the problems of the ICU message raw,
// which tokens holds the tokens of, in locale.
func Analyze(
	locale language.Tag, raw string, tokens []icumsg.Token,
	selectOptions icumsg.SelectOptions,
) (problems []Problem) {
	for err := range icumsg.Errors(locale, raw, tokens, selectOptions) {
		switch err := err.(type) {
		case icumsg.ErrorPluralMissingOption:
			problems = append(problems, Problem{
				Arg:     tokens[err.TokenIndex+1].String(raw, tokens),
				Missing: slices.Collect(err.MissingOptions()),
			})
		case icumsg.ErrorSelectMissingOption:
			problems = append(problems, Problem{
				Arg:     tokens[err.TokenIndex+1].String(raw, tokens),
				Missing: slices.Collect(err.MissingOptions()),
			})
		case icumsg.ErrorSelectInvalidOption:
			problems = append(problems, Problem{
				Arg:     tokens[err.TokenIndexArgument+1].String(raw, tokens),
				Invalid: tokens[err.TokenIndexOption].String(raw, tokens),
			})
		}
	}
	return problems
}

// AnalysisReport describes each problem that Analyze returns in English.
func AnalysisReport(
	locale language.Tag, raw string, tokens []icumsg.Token,
	selectOptions icumsg.SelectOptions,
) (report []string) {
	for _, p := range Analyze(locale, raw, tokens, selectOptions) {
		report = append(report, p.String())
	}
	return report
}
