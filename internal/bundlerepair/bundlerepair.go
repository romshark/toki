// Package bundlerepair repairs corrupt native locale bundle messages
// (see [codeparse.CatalogStatistics.MessagesCorrupt]) by regenerating
// the ICU from the TIK.
package bundlerepair

import (
	"fmt"
	"strings"

	"github.com/romshark/icumsg"
	tik "github.com/romshark/tik/tik-go"
	"github.com/romshark/toki/internal/arb"
	"github.com/romshark/toki/internal/codeparse"
	tikutil "github.com/romshark/toki/internal/tik"
)

// Repaired describes a single repaired message.
type Repaired struct {
	TIKID      string
	ICUMessage string
}

// Repair fixes corrupt native locale messages in the scan by fully
// regenerating them from the TIK (ICU text, placeholders, metadata).
// It modifies catalog.ARB.Messages in place.
// The caller is responsible for writing ARB files to disk afterward.
func Repair(
	scan *codeparse.Scan,
	tikICUTranslator *tik.ICUTranslator,
	icuTokenizer *icumsg.Tokenizer,
) []Repaired {
	var nativeCatalog *codeparse.Catalog
	for c := range scan.Catalogs.Seq() {
		if c.ARB.Locale == scan.DefaultLocale {
			nativeCatalog = c
			break
		}
	}
	if nativeCatalog == nil {
		return nil
	}

	var repaired []Repaired
	for _, i := range scan.TextIndexByID.SeqRead() {
		t := scan.Texts.At(i)
		expectedICU := tikICUTranslator.TIK2ICU(t.TIK)

		msg := nativeCatalog.ARB.Messages[t.IDHash]
		corrupt := false
		switch {
		case msg.ICUMessage == "":
			corrupt = true
		case tikutil.ProducesCompleteICU(scan.DefaultLocale, t.TIK) &&
			msg.ICUMessage != expectedICU:
			corrupt = true
		case codeparse.PlaceholdersMismatch(t.TIK, msg):
			corrupt = true
		}
		if !corrupt {
			continue
		}

		// Rebuild the entire message from the TIK.
		newMsg := newARBMessage(t, expectedICU, scan, icuTokenizer)
		nativeCatalog.ARB.Messages[t.IDHash] = newMsg
		repaired = append(repaired, Repaired{TIKID: t.IDHash, ICUMessage: expectedICU})
	}
	nativeCatalog.MessagesCorrupt.Store(0)
	return repaired
}

// newARBMessage builds a complete arb.Message from a codeparse.Text.
func newARBMessage(
	text codeparse.Text, icuMsg string,
	scan *codeparse.Scan, icuTokenizer *icumsg.Tokenizer,
) arb.Message {
	placeholders := make(map[string]arb.Placeholder)
	for i, ph := range text.TIK.Placeholders() {
		var pl arb.Placeholder
		name := fmt.Sprintf("var%d", i)
		switch ph.Type {
		case tik.TokenTypeText:
			pl.Description = "arbitrary string"
			pl.Type = arb.PlaceholderString
		case tik.TokenTypeTextWithGender:
			pl.Description = "arbitrary string with gender information"
			pl.Type = arb.PlaceholderString
		case tik.TokenTypeCardinalPluralStart:
			pl.Description = "cardinal plural"
			pl.Type = arb.PlaceholderNum
			pl.Example = "2"
		case tik.TokenTypeOrdinalPlural:
			pl.Description = "ordinal plural"
			pl.Type = arb.PlaceholderNum
			pl.Example = "4"
		case tik.TokenTypeDateFull,
			tik.TokenTypeDateLong,
			tik.TokenTypeDateMedium,
			tik.TokenTypeDateShort:
			pl.Description = "date"
			pl.IsCustomDateFormat = true
			pl.Type = arb.PlaceholderDateTime
		case tik.TokenTypeTimeFull,
			tik.TokenTypeTimeLong,
			tik.TokenTypeTimeMedium,
			tik.TokenTypeTimeShort:
			pl.Description = "time"
			pl.IsCustomDateFormat = true
			pl.Type = arb.PlaceholderDateTime
		case tik.TokenTypeCurrency:
			pl.Description = "currency with amount"
			pl.Type = arb.PlaceholderNum
			pl.Example = "USD(4.00)"
		}
		placeholders[name] = pl
	}

	var icuTokens []icumsg.Token
	tokens, err := icuTokenizer.Tokenize(scan.DefaultLocale, nil, icuMsg)
	if err == nil {
		icuTokens = tokens
	}

	return arb.Message{
		ID:               text.IDHash,
		ICUMessage:       icuMsg,
		ICUMessageTokens: icuTokens,
		Description:      strings.Join(text.Comments, " "),
		Type:             arb.MessageTypeText,
		Context:          text.Context(),
		Placeholders:     placeholders,
	}
}
