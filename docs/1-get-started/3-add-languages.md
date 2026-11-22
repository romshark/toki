# Add languages

Run the generator with `-t` for each new locale:

```sh
go run github.com/romshark/toki@latest generate -t en-US -t de -t fr
```

This creates 3 new catalogs: American English (`en-US`), German (`de`) and French (`fr`). Every message of a new catalog starts blank.

Provide your translators with the new `.arb` files, or translate the messages yourself in the `.arb` files or with `toki edit`. Once you have the translated `.arb` files, replace the ones in the bundle package and rerun the generator to update it:

```sh
go run github.com/romshark/toki@latest generate
```

Toki rejects plural options that the locale of a catalog doesn't have, and reports how complete each catalog is. Add `-v` to list the options that each message is missing.

> [!TIP]
> If you write your TIKs in a language other than English, the [CLDR plural rules table](https://www.unicode.org/cldr/charts/47/supplemental/language_plural_rules.html) shows the plural options of each language.

Next, check your translations in [continuous integration](4-ci.md).
