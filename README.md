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
files are used as intermediate translation storage for
[ICU messages](https://unicode-org.github.io/icu/userguide/format_parse/messages/)
generated from [TIKs](https://github.com/romshark/tik) extracted from the source code.


## Quick Start Guide

1. Make yourself familiar with
   [Textual Internationalization Key](https://github.com/romshark/tik) syntax.
2. Create new project directory `mkdir tokiexample && cd tokiexample`.
3. Initialize the Go module by running `go mod init tokiexample && go mod tidy`.
4. Run `go run github.com/romshark/toki@latest -l en && go mod tidy`
   which will create a new Toki bundle package with default language set to `en`.
5. Import the bundle package in your program and add some TIKs:

```go
package main

import (
	"fmt"

	"tokiexample/tokibundle"

	"golang.org/x/text/language"
)

func main() {
	// Get a localized reader for British English.
	// Toki will automatically select the most appropriate translation catalog available.
	reader, _ := tokibundle.Match(language.BritishEnglish)

	// This comment describes the text below and is included in the translator context.
	fmt.Println(reader.String(`{"Framework"} is powerful yet easy to use!`, "Toki"))
}
```

6. Run `go run github.com/romshark/toki@latest -l en` again to update the bundle.
7. Done! Your setup is now ready and you can run your program with `go run .`.

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
