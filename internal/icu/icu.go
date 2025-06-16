package icu

import (
	"fmt"
	"slices"
	"strings"

	"github.com/romshark/icumsg"
	"github.com/romshark/icumsg/cldr"
	"golang.org/x/text/language"
)

type SelectOptions func(argName string) (
	[]string, icumsg.OptionsPresencePolicy, icumsg.OptionUnknownPolicy,
)

func CompletenessReport(
	locale language.Tag, raw string, tokens []icumsg.Token, selectOptions SelectOptions,
) (report []string) {
	_ = icumsg.Completeness(
		raw, tokens, locale,
		selectOptions,
		func(index int) {
			// On incomplete.
			tok := tokens[index]
			argNameTok := tokens[index+1]
			argName := argNameTok.String(raw, tokens)
			var missingOpts string
			switch tok.Type {
			case icumsg.TokenTypePlural:
				need, _ := cldr.LocalePluralRules(locale)
				has := findAllPluralOptions(tokens, index)
				missingOpts = missingOptions(need, has)
			case icumsg.TokenTypeSelectOrdinal:
				_, need := cldr.LocalePluralRules(locale)
				has := findAllPluralOptions(tokens, index)
				missingOpts = missingOptions(need, has)
			case icumsg.TokenTypeSelect:
				need, _, _ := selectOptions(argName)
				has := findAllSelectOptions(raw, tokens, index)
				missingOpts = missingSelectOptions(need, has)
			}
			if missingOpts != "" {
				s := fmt.Sprintf("incomplete %s: %s: missing options: %s",
					tok.Type.String(), argName, missingOpts)
				report = append(report, s)
			}
		},
		func(index int) { /* Rejected, no-op. */ },
	)
	return report
}

func missingOptions(need, has cldr.PluralRules) string {
	var b strings.Builder
	b.WriteByte('[')
	sep := ""
	if need.Zero && !has.Zero {
		b.WriteString(sep)
		b.WriteString("zero")
		sep = ","
	}
	if need.One && !has.One {
		b.WriteString(sep)
		b.WriteString("one")
		sep = ","
	}
	if need.Two && !has.Two {
		b.WriteString(sep)
		b.WriteString("two")
		sep = ","
	}
	if need.Few && !has.Few {
		b.WriteString(sep)
		b.WriteString("few")
		sep = ","
	}
	if need.Many && !has.Many {
		b.WriteString(sep)
		b.WriteString("many")
	}
	// No need to check "other" since it's always required.
	// A message that doesn't specify other is invalid.
	b.WriteByte(']')
	return b.String()
}

func missingSelectOptions(need, has []string) string {
	var b strings.Builder
	b.WriteByte('[')
	sep := ""
	for _, n := range need {
		if slices.Index(has, n) == -1 {
			b.WriteString(sep)
			b.WriteString(n)
			sep = ","
		}
	}
	b.WriteByte(']')
	return b.String()
}

func findAllPluralOptions(tokens []icumsg.Token, index int) (has cldr.PluralRules) {
	for t := range icumsg.Options(tokens, index) {
		switch tokens[t].Type {
		case icumsg.TokenTypeOptionZero:
			has.Zero = true
		case icumsg.TokenTypeOptionOne:
			has.One = true
		case icumsg.TokenTypeOptionTwo:
			has.Two = true
		case icumsg.TokenTypeOptionFew:
			has.Few = true
		case icumsg.TokenTypeOptionMany:
			has.Many = true
		case icumsg.TokenTypeOptionOther:
			has.Other = true
		}
	}
	return has
}

func findAllSelectOptions(msg string, tokens []icumsg.Token, index int) (has []string) {
	for t := range icumsg.Options(tokens, index) {
		switch tokens[t].Type {
		case icumsg.TokenTypeOptionOther:
			has = append(has, "other")
		case icumsg.TokenTypeOption:
			has = append(has, tokens[t+1].String(msg, tokens))
		}
	}
	return has
}
