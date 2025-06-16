package main

import (
	"flag"
	"fmt"
	"time"

	"tokiexample/tokibundle"

	"golang.org/x/text/language"
)

func main() {
	fLocale := flag.String("l", "en", "i18n locale")
	flag.Parse()
	locale := language.MustParse(*fLocale)

	fmt.Println("Supported locales:", tokibundle.Locales())

	// Get a localized reader for British English.
	// Toki will automatically select the most appropriate available translation catalog.
	reader, conf := tokibundle.Match(locale)
	fmt.Println("Selected", reader.Locale(), "with confidence:", conf)

	fmt.Println(reader.String("translated text"))
	fmt.Println(reader.String("Nothing found in folder {text}", "images"))

	fmt.Println(reader.String("It was finished on {date-full} at {time-full}",
		time.Now(), time.Now()))

	fmt.Println(reader.String("{# projects were} finished on {date-full} at {time-full} by {name}",
		4, time.Now(), time.Now(), tokibundle.String{Value: "Rafael", Gender: tokibundle.GenderMale}))

	fmt.Println(reader.String("searched {# files} in {# folders}", 56, 21))
}
