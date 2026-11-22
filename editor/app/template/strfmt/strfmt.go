// Package strfmt provides text formatting helpers for templates.
package strfmt

import "strings"

func SingleQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	b.WriteString(s)
	b.WriteByte('\'')
	return b.String()
}

// Bool returns "true" or "false".
func Bool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
