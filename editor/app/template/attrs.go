package template

import (
	"maps"

	"github.com/a-h/templ"
)

// attrs merges attribute sets left to right; later sets override earlier
// ones on key collision. Morpheus components take their passthrough
// attributes as a single templ.Attributes, so call sites that combine a
// fixed set (classes, ARIA) with a caller-supplied set merge them here
// rather than assembling maps inline.
func attrs(sets ...templ.Attributes) templ.Attributes {
	merged := templ.Attributes{}
	for _, s := range sets {
		maps.Copy(merged, s)
	}
	return merged
}

// hiddenWhen renders an inline display:none for the server-rendered
// initial state of an element whose visibility Datastar then owns via
// data-show.
func hiddenWhen(hidden bool) templ.Attributes {
	if !hidden {
		return nil
	}
	return templ.Attributes{"style": "display:none"}
}

// fontFamilyStyle returns the font-family declaration for a font preview,
// or nil when the option inherits the page font.
//
// The family must not contain quotes. templ escapes style attribute values
// twice — the sanitiser turns ' into &#39; and the attribute writer turns
// the & into &amp; — so a quoted name like 'Times New Roman' reaches the
// browser as literal &amp;#39;, invalidating the declaration and silently
// falling back to the inherited font. CSS accepts unquoted family names
// made of identifiers, which is what the font tables use. SafeCSSProperty
// does not help: that path escapes too.
func fontFamilyStyle(family string) map[string]templ.SafeCSSProperty {
	if family == "" {
		return nil
	}
	return map[string]templ.SafeCSSProperty{
		"font-family": templ.SafeCSSProperty(family),
	}
}

// fontSizeStyle returns the sample's font-size (and family, when the
// section previews in a specific face). Same no-quotes constraint as
// fontFamilyStyle — see there for why.
func fontSizeStyle(sz FontSizeOption) map[string]templ.SafeCSSProperty {
	style := map[string]templ.SafeCSSProperty{
		"font-size": templ.SafeCSSProperty(sz.Size),
	}
	if sz.Family != "" {
		style["font-family"] = templ.SafeCSSProperty(sz.Family)
	}
	return style
}

// fieldLabelID derives the id of a field's visible label from the signal
// the field writes, so the control can reference it with aria-labelledby.
func fieldLabelID(signalName string) string { return signalName + "-label" }
