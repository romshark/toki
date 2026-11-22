package app

import (
	"net/http"
	"slices"
	"strings"

	"github.com/romshark/datapages"

	"github.com/romshark/toki/editor/app/template"
)

// sectionCookie names the cookie that keeps whether the user closed the
// collapsible section key.
func sectionCookie(key string) string { return "toki-section-" + key }

// readClosedSections returns which collapsible sections the cookies of r mark
// closed, for every section. A nil map would leave the sections unset.
func readClosedSections(r *http.Request) map[string]bool {
	closed := make(map[string]bool, len(template.CollapsibleSections))
	for _, key := range template.CollapsibleSections {
		c, err := r.Cookie(sectionCookie(key))
		closed[key] = err == nil && c.Value == "closed"
	}
	return closed
}

// setSectionOpen keeps in the cookie of collapsible section key whether the
// user opened it, for the next page load. The POSTSetSectionOpen action of
// each page with collapsible sections calls it.
func setSectionOpen(sse datapages.SSE, key string, open bool) error {
	if !slices.Contains(template.CollapsibleSections, key) {
		return datapages.ErrBadRequest
	}
	value := "closed"
	if open {
		value = "open"
	}
	var script strings.Builder
	writeUIPrefCookieScript(&script, sectionCookie(key), value)
	return sse.ExecuteScript(script.String())
}
