<a href="https://pkg.go.dev/github.com/romshark/toki">
    <img src="https://godoc.org/github.com/romshark/toki?status.svg" alt="GoDoc">
</a>
<a href="https://goreportcard.com/report/github.com/romshark/toki">
    <img src="https://goreportcard.com/badge/github.com/romshark/toki" alt="GoReportCard">
</a>
<a href='https://coveralls.io/github/romshark/toki?branch=main'>
    <img src='https://coveralls.io/repos/github/romshark/toki/badge.svg?branch=main&service=github' alt='Coverage Status' />
</a>

# Toki

Toki is an i18n framework for Go and a Textual Internationalization Key
([**TIK**](https://github.com/romshark/tik))
processor implementation.

`toki generate` parses the source code, lints it reporting any misuse or errors,
extracts localized texts and generates a localization bundle.

[app-resource-bundle (.arb)](https://github.com/google/app-resource-bundle)
files as used as intermediate translation storage,
[TIK](https://github.com/romshark/tik) for text keys and
[ICU messages](https://unicode-org.github.io/icu/userguide/format_parse/messages/).

## Quick Start Guide

1. Make yourself familiar with
   [Textual Internationalization Key](https://github.com/romshark/tik) syntax.
2. Write a Go program with localized texts
   (`reader.String` will return localized strings):

```go
package main

import (
	"fmt"

	"github.com/romshark/toki"
	"golang.org/x/text/language"
)

func main() {
	// Make a new localizer with English being the default language.
	localization, err := toki.New(language.MustParse("en"), nil)
	if err != nil {
		panic(fmt.Errorf("initializing localization bundle: %w", err))
	}

	// Get a localized reader for British English.
	// Toki will automatically select the most appropriate translation catalog available.
	reader, _ := localization.Match(language.BritishEnglish)

	// This comment describes the text below and is included in the translator context.
	fmt.Println(reader.String(`{"Framework"} is powerful yet easy to use!`, "Toki"))
}
```

3. Run `toki generate`

```
go run github.com/romshark/toki/cmd/toki@v0.2.0 generate -l en -b path/to/myi18nbundle
```

This will create a new Go package under `./path/to/` named `myi18nbundle` with `en` (English)
as source code language containing your generated Go i18n code and translation catalogs.

4. Import the generated bundle package into your application and pass it to `toki.New`:

```go
package main

import (
  "fmt"

  "github.com/romshark/toki"
  "golang.org/x/text/language"
  
  "yourmodule/myi18nbundle"
)

func main() {
  // Make a new localizer with English being the default language.
  localization, err := toki.New(language.MustParse("en"), myi18nbundle.Bundle{})
  if err != nil {
    panic(fmt.Errorf("initializing localization bundle: %w", err))
  }
  //...
```

5. Done! Your setup is now ready.

## Bundle File Structure

- `bundle_gen.go` contains the generated `Bundle` type, helper functions and
  overwritable fallback functions (`MissingTranslationString`, `MissingTranslationWrite`).
  `Bundle` contains all catalogs and implements the `toki.Bundler` interface.
  - **Not editable** 🤖 Any manual change is always overwritten.
- `catalog_<locale>_gen.go` contains the catalog type for a particular locale
  implementing the `toki.Reader` interface.
  - **Not editable** 🤖 Any manual change is always overwritten.
- `catalog.<locale>.arb` is an app resource bundle file containing actual translations
  for a particular locale.
  - **Editable 📝**
  - Changed translations are preserved.
  - If a new text isn't found in the translation file it's automatically added.
  - If a text is no longer used in the source code it's removed from this file.
- `head.txt` is a text file defining the head comment to use in generated files.
  - **Editable 📝**
  - If this file isn't found a new blank file is always automatically created.
- `context.txt` is a text file defining the overall global context for translators.
  - **Editable 📝**
  - If this file isn't found a new blank file is always automatically created.

All other files in the bundle package are ignored.
