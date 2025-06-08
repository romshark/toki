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

### 1. Create a new Go project.

```sh
mkdir tokiexample && cd tokiexample;
go mod init tokiexample;
```

**tokiexample/main.go**
```go
package main

import (
	"fmt"
	"time"
)

func main() {
	now, numMsgs := time.Now(), 42
	fmt.Printf(
		"It's %s and you have %d message(s)\n",
		now.Format("Monday, Jan 2, 2006"), numMsgs,
	)
}
```

### 2. Prepare your Go project for internationalization.

1. First, we need to generate a new Toki bundle package
with default language set to `en` (English):

```sh
go run github.com/romshark/toki@latest generate -l en && go mod tidy
```

2. Second, you'll need to adjust your messages in the source code to TIKs.
Use the [TIK cheatsheet](https://romshark.github.io/tik-cheatsheet/)
for help and guidance.

```go
package main

import (
	"fmt"

	"tokiexample/tokibundle"

	"golang.org/x/text/language"
)

func main() {
	now, numMsgs := time.Now(), 42

	// Get a localized reader for British English.
	// Toki will automatically select the most appropriate translation catalog available.
	reader, _ := tokibundle.Match(language.BritishEnglish)

	// This comment describes the text below and is included in the translator context.
	fmt.Println(reader.String(
		`It's {Friday, July 16, 1999} and you have {2 messages}`,
		now, numMsgs,
	))
}
```

3. Now regenerate your Toki bundle to include the new localized message:

```sh
go run github.com/romshark/toki@latest generate
```

This will update your `catalog_en.arb` file and add the new message to it.
However, Toki can't translate your messages fully so you will see in the generator report
that your translations for `en` are incomplete yet.

To manually complete the translation, add the missing `one {# message}`
[plural option](https://cldr.unicode.org/index/cldr-spec/plural-rules) to the
[ICU message](https://unicode-org.github.io/icu/userguide/format_parse/messages/)
such that it becomes:

```
"msga5a0f2138b9d6598": "It''s {var0, date, full} and you have {var1, plural, one{# message} other {# messages}}",
```

You can also make it a bit better by adding a special case for `0`:

```
"msga5a0f2138b9d6598": "It''s {var0, date, full} and you have {var1, plural, =0{no messages} one{# message} other {# messages}}",
```

4. After tweaking the catalog files, rerun the generator to update your bundle once again:

```sh
go run github.com/romshark/toki@latest generate
```

### 3. Localize your application for other languages and regions.

1. Run the generator, but this time, add a new parameter `-t en-US -t de -t fr`

```sh
go run github.com/romshark/toki@latest generate -t en-US -t de -t fr
```

This will create three new catalogs for
American English (`en-US`), German (`de`) and French (`fr`).

2. You can now provide your translators with the new generated `.arb` files
   or repeat the same procedure as we did for `en` yourself.

3. Once you get your hands on translated `.arb` files, simply replace the ones in the
   Toki bundle package and rerun the generator to update it:

```sh
go run github.com/romshark/toki@latest generate
```

**Coming Soon: LLM-based auto-translator.**

**TIP:** If you're writing your TIKs in a different language than English you may find the
[CLDR plural rules table](https://www.unicode.org/cldr/charts/47/supplemental/language_plural_rules.html)
helpful. Don't worry, Toki will make sure you're not using illegal options for your
default locale and will report any missing options.

### 4. Integrate Toki into your CI/CD pipeline.

You've now (mostly) mastered the entire i18n workflow for Toki.
However, to make sure you never deploy broken or unfinished localizations,
add Toki to your CI/CD setup:

```sh
go run github.com/romshark/toki@latest lint -require-complete
```

You may also use `toki generate -require-complete` and additionally git diff
to ensure your generated Toki bundle package is up to date.

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
