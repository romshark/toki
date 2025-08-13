package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/joeblew999/toki/examples/simpleweb/tokibundle"
	"golang.org/x/text/language"
)

func main() {
	http.HandleFunc("/", handleRoot)

	fmt.Println("🌐 Simple web server listening on :8080")
	fmt.Println("📍 Try: http://localhost:8080")
	fmt.Println("📍 Try: http://localhost:8080/?lang=de")
	fmt.Println("📍 Try: http://localhost:8080/?lang=ru")
	fmt.Println("📍 Try: curl -H 'Accept-Language: de,en;q=0.9' http://localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	// Get locale from URL param first, then Accept-Language header
	locale := getBestLocale(r)

	// Get reader for detected locale
	reader, confidence := tokibundle.Match(locale)

	// Build response
	fmt.Fprintf(w, "<!DOCTYPE html>")
	fmt.Fprintf(w, "<html><head><title>Toki Web Example</title></head><body>")
	fmt.Fprintf(w, "<h1>%s</h1>", reader.String("translated text"))
	fmt.Fprintf(w, "<p><strong>Detected Locale:</strong> %s</p>", reader.Locale())
	fmt.Fprintf(w, "<p><strong>Confidence:</strong> %s</p>", confidence)
	fmt.Fprintf(w, "<p><strong>Available Locales:</strong> %s</p>", tokibundle.Locales())
	fmt.Fprintf(w, "<p><strong>File Not Found:</strong> %s</p>", reader.String("Nothing found in folder {text}", "images"))
	fmt.Fprintf(w, "<p><strong>Search Results:</strong> %s</p>", reader.String("searched {# files} in {# folders}", 42, 7))
	fmt.Fprintf(w, "</body></html>")
}

func getBestLocale(r *http.Request) language.Tag {
	// Priority 1: URL parameter
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return language.MustParse(lang)
	}

	// Priority 2: Accept-Language header
	if accept := r.Header.Get("Accept-Language"); accept != "" {
		// Parse Accept-Language header
		prefs, _, err := language.ParseAcceptLanguage(accept)
		if err == nil && len(prefs) > 0 {
			return prefs[0]
		}
	}

	// Priority 3: Default
	return language.English
}
