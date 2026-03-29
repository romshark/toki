// Package strfmt provides text formatting helpers for templates.
package strfmt

import (
	"fmt"
	"strings"
)

type Integer interface {
	uint | int | uint8 | int8 | uint16 | int16 | uint32 | int32 | uint64 | int64
}

// Int formats an integer with thousands separators (e.g. 10,000).
func Int[I Integer](i I) string {
	s := fmt.Sprintf("%d", i)
	if len(s) <= 3 {
		return s
	}
	// Insert commas from right to left.
	start := 0
	if s[0] == '-' {
		start = 1
	}
	digits := s[start:]
	var b strings.Builder
	b.Grow(len(s) + len(digits)/3)
	b.WriteString(s[:start])
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// Percent formats a 0.0–1.0 ratio as a percentage string with up to
// 2 decimal places, trimming trailing zeros (e.g. "0.01%", "12.5%", "100%").
// Shows "<0.01%" when positive but below the display threshold.
func Percent(ratio float64) string {
	pct := ratio * 100
	if pct > 0 && pct < 0.01 {
		return "<0.01%"
	}
	s := fmt.Sprintf("%.2f", pct)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s + "%"
}

// Bool returns "true" or "false".
func Bool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ChangeWord returns "change" for 1, "changes" otherwise.
func ChangeWord(n int) string {
	if n == 1 {
		return "change"
	}
	return "changes"
}
