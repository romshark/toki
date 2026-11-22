# Add languages

Run the generator with `-t` for each new locale:

```sh
go run github.com/romshark/toki@latest generate -t en-US -t de -t fr
```

This creates 3 new catalogs: American English (`en-US`), German (`de`) and French (`fr`).

Provide your translators with the new `.arb` files, or [complete the translations](1-quick-start.md#complete-the-translation) yourself as for `en`. Once you have the translated `.arb` files, replace the ones in the bundle package and rerun the generator to update it:

```sh
go run github.com/romshark/toki@latest generate
```

> [!NOTE]
> Coming soon: an LLM-based auto-translator.

> [!TIP]
> If you write your TIKs in a language other than English, the [CLDR plural rules table](https://www.unicode.org/cldr/charts/47/supplemental/language_plural_rules.html) can help. Toki makes sure you don't use illegal options for your default locale and reports any missing options.

Next, check your translations in [continuous integration](3-ci.md).
