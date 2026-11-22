---
suggested: true
---

# Quick start

## Create a Go project

```sh
mkdir tokiexample && cd tokiexample
go mod init tokiexample
```

```go title="main.go"
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

## Generate a bundle

Generate a new Toki bundle package with the default language `en` (English):

```sh
go run github.com/romshark/toki@latest generate -l en && go mod tidy
```

## Write TIKs

Adjust the messages in your source code to TIKs. Use the [TIK cheatsheet](https://romshark.github.io/tik/cheatsheet/) for help and guidance.

```go title="main.go"
package main

import (
	"fmt"
	"time"

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
		`It's {date-full} and you have {# messages}`,
		now, numMsgs,
	))
}
```

Regenerate the bundle to include the new localized message:

```sh
go run github.com/romshark/toki@latest generate
```

This adds the new message to `tokibundle/catalog_en.arb`. Toki can't translate your messages fully: the message has only the `other` plural option, and the generator report shows that the translation for `en` is still incomplete.

```
"msgbe2fefc496f64dfb": "It''s {var0, date, full} and you have {var1, plural, other {# messages}}",
```

## Complete the translation

To complete the translation, add the missing `one {# message}` [plural option](https://cldr.unicode.org/index/cldr-spec/plural-rules) to the [ICU message](https://unicode-org.github.io/icu/userguide/format_parse/messages/) in `catalog_en.arb`:

```
"msgbe2fefc496f64dfb": "It''s {var0, date, full} and you have {var1, plural, one{# message} other {# messages}}",
```

You can also make it a bit better by adding a special case for `0`:

```
"msgbe2fefc496f64dfb": "It''s {var0, date, full} and you have {var1, plural, =0{no messages} one{# message} other {# messages}}",
```

> [!TIP]
> The double apostrophe `''` is not a typo. It's [ICU's escape for a single quote](https://unicode-org.github.io/icu/userguide/format_parse/messages/#quotingescaping).

> [!TIP]
> Use `toki edit` to open the native desktop editor for translations, or `toki edit -server <addr>` to run it as a plain HTTP server.

After editing the catalog files, rerun the generator to update your bundle:

```sh
go run github.com/romshark/toki@latest generate
```

Next, [add languages](2-add-languages.md) to your program.
