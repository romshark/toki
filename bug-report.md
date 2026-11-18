# Toki v0.8.5 bug report

Toki v0.8.5 has 7 defects found while building `example/i18n-toki` in Datapages. Gendered translations panic at runtime. `=0` plural options are dropped. `Match` serves a non-default catalog to visitors whose language the bundle lacks. The other 4 are workflow defects. Line numbers refer to the `v0.8.5` tag.

| # | Severity | Defect |
| - | -------- | ------ |
| 1 | High | A gender `select` in a translation panics at runtime |
| 2 | High | `Match` returns an arbitrary catalog instead of `DefaultLocale` when nothing matches |
| 3 | High | The ARB decoder rejects `varN_gender`, which fails `toki generate` for the whole bundle |
| 4 | Medium | Exact plural options such as `=0` are dropped from generated code |
| 5 | Medium | The README Quick Start command fails on the first run |
| 6 | Low | `toki generate` rewrites every bundle file on every run |
| 7 | Low | The generated-file header does not match the Go convention |

Environment: Go 1.27.3 darwin/arm64, `go run github.com/romshark/toki@v0.8.5`, `github.com/go-playground/locales` v0.14.2, `golang.org/x/text` v0.43.0, bundle locales `ar`, `de`, `en` (default), `fr`, `ru`.

## 1. A gender select in a translation panics

`writeSelect` (`internal/gengo/iowriter.go:302`) parses the argument name with `parseArgName`, which sets `argName.Gender` for a `_gender` suffix (`internal/gengo/gengo.go:253-276`). Nothing reads that field. Line 310 always emits a type assertion to `string` on the base argument:

```go
w.printf("switch args[%d].(string) {\n", argIndex.Index)
```

A `{name}` placeholder takes a `tokibundle.String`, as the TIK specification and the README show. The assertion fails:

```
panic: interface conversion: interface {} is tokibundle.String, not string

tokiprobe/tokibundle.init.func14(...)
	tokibundle/catalog_ar_gen.go:215
```

A plain `string` argument does not panic, but the switch then compares the name itself, such as `"Ana"`, with `"male"` and `"female"`, and always takes the `other` branch. Gendered translations therefore cannot work in v0.8.5, which leaves the `Gender` type and `tokibundle.String` without effect.

Reproduce with a TIK in source:

```go
reader.String("{name} has signed up",
	tokibundle.String{Value: "Ana", Gender: tokibundle.GenderFemale})
```

and this translation in `catalog_fr.arb`, with the placeholder workaround from defect 3:

```json
"msg9fa2f8e5bc268f46": "{var0_gender, select, female {{var0} s’est inscrite} male {{var0} s’est inscrit} other {{var0} participe}}"
```

Expected `Ana s’est inscrite`, got the panic above.

Fix: for a gender argument, switch on the gender that `sv` returns and map the option names to the constants:

```go
if argIndex.Gender {
	w.printf("switch _, g := sv(args[%d]); g {\n", argIndex.Index)
	// option "male" -> case GenderMale:, "female" -> case GenderFemale:, other -> default:
}
```

A test that renders a `_gender` select for each of the 3 genders would have caught it. `internal/gengo` has no tests in v0.8.5. The `_gender` cases in `internal/icu/icu_test.go` test the completeness analysis, not generated code.

## 2. Match returns an arbitrary catalog when nothing matches

`Match` (`internal/gengo/template.go.txt:334-344`) ignores the confidence that `language.Matcher.Match` returns:

```go
t, _, c := matcher.Match(locales...)
for t := t; t != language.Und; t = t.Parent() {
	s := localeStringCache.String(t)
	if r, ok := readerByLocale[s]; ok {
		return r, c
	}
}
// Fallback to default.
return readerByLocale[DefaultLocale], language.No
```

The matcher is built from `allLocales` (`template.go.txt:242`), which `Catalogs()` fills in alphabetical order. `language.NewMatcher` treats its first tag as the default, which is `ar` in this bundle, not `DefaultLocale`. With no match the matcher still returns a supported tag, the loop finds its catalog, and the fallback on line 343 is unreachable.

Measured with the 5-locale bundle:

| `Match` arguments | Returned catalog | Confidence |
| ----------------- | ---------------- | ---------- |
| none | `ar` | `No` |
| `ja` | `ar` | `No` |
| `pt-BR` | `de` | `No` |
| `nb` | `de` | `No` |
| `de-AT` | `de` | `High` |
| `en-GB` | `en` | `High` |

Every visitor whose `Accept-Language` the bundle has no catalog for, and every request without the header, is served Arabic. The README promises the most appropriate catalog (`README.md:90`).

Fix: return the default catalog when the confidence is `language.No`, and build the matcher with `DefaultLocale` first:

```go
t, _, c := matcher.Match(locales...)
if c == language.No {
	return readerByLocale[DefaultLocale], language.No
}
```

Applications on v0.8.5 have to check the confidence themselves. `example/i18n-toki/app/locale.go` does.

## 3. The ARB decoder rejects varN_gender

For a TIK with `{name}`, Toki writes placeholder metadata for `var0` only:

```json
"@msg9fa2f8e5bc268f46": {
	"placeholders": {
		"var0": {"type": "String", "description": "arbitrary string with gender information"}
	}
}
```

A translation that uses the gender as the TIK specification describes (`{var0_gender, select, ...}`) then fails in the decoder (`internal/arb/arb.go:283-289`), which requires every argument name to be in the metadata. The error stops `toki generate` for the whole bundle, not only for that catalog:

```
[ERROR] analyzing sources: searching .arb files: parsing .arb file: undefined placeholder: "var0_gender"
```

Workaround: add the entry by hand. `toki generate` keeps it on later runs:

```json
"var0_gender": {"type": "String", "description": "grammatical gender of var0"}
```

Fix: write `varN_gender` metadata for each `{name}` placeholder, or let the decoder accept `varN_gender` when `varN` is declared with type `String`. Name the message and the catalog file in the error. The current message names neither.

## 4. Exact plural options are dropped

`writePlural` (`internal/gengo/iowriter.go:181-257`) collects the options `zero` to `other` and emits 1 `switch` over the plural rule. icumsg tokenizes `=0` as `TokenTypeOptionNumber`, which the loop at lines 206-221 has no case for, so the option is silently omitted. `toki generate` reports the catalog as 100% complete.

```json
"msg5a4e865f6503c073": "{var0, plural, =0 {Fully booked} one {# seat left} other {# seats left}}"
```

renders `0 seats left` for 0, not `Fully booked`. The same holds in every locale. In `ar`, 0 takes the `zero` category branch instead of `=0`. The README recommends `=0{no messages}` (`README.md:123`).

Fix: emit the exact-value options before the rule switch. In ICU an explicit `=N` option takes precedence over the keyword categories and compares the argument before `offset` is subtracted.

## 5. The Quick Start command fails on the first run

`README.md:68`:

```sh
go run github.com/romshark/toki@latest generate -l en && go mod tidy
```

In a module that does not require `github.com/go-playground/locales` and `golang.org/x/text`, `generate` writes `bundle_gen.go`, then fails to load the new package and exits with status 1:

```
[ERROR] analyzing sources: errors in package "tokibundle": [.../bundle_gen.go:12:2: could not import github.com/go-playground/locales (invalid package name: "") ...]
exit status 1
```

`&&` then skips `go mod tidy`. The catalogs requested with `-t` are not created either. The next steps work only after a manual `go mod tidy` and a second `generate`.

Fix: add the requirements of a new bundle with `go get` before loading packages, or skip loading the bundle package on the run that creates it. Otherwise document `generate -l en; go mod tidy; generate`.

## 6. Every run rewrites every bundle file

`toki generate` writes all `catalog_*.arb` and `*_gen.go` files on every run, also when their content does not change. Running it 2 times without a source change updated the modification times of all 11 files, `bundle_gen.go`, 5 `.arb` and 5 `catalog_*_gen.go`. `@@last_modified` stayed the same.

A file watcher that runs `toki generate` when a `.arb` file changes therefore triggers itself without end. This prevents a `datapages watch` custom watcher on `tokibundle/*.arb`, which the example would otherwise use to apply translator edits.

Fix: compare with the file on disk and skip the write when the bytes are equal.

## 7. The header is not a Go generated-code header

Generated files start with:

```go
// Generated by github.com/romshark/toki. DO NOT EDIT.
```

The Go convention is `^// Code generated .* DO NOT EDIT\.$` (https://go.dev/s/generatedcode). `go/ast.IsGenerated` returns false for `bundle_gen.go` and `catalog_en_gen.go`, and true for a templ `_templ.go` file. Tools that detect generated code by this convention treat the bundle as hand-written.

Fix: write `// Code generated by github.com/romshark/toki. DO NOT EDIT.` The README states which header marks a file as Toki's own ("Bundle File Structure"). `toki generate` has to keep recognizing the old header to overwrite bundles written by v0.8.5.

## Not a Toki defect

`{currency}` with `currency.EUR` renders `EUR12.50` for `en`, while `de`, `fr`, `ru` and `ar` render `12,50 €` or `12٫50 €`. The `en` table of `github.com/go-playground/locales` v0.14.2 holds `EUR` as the symbol (`en/en.go:58`), where CLDR has `€`. Toki inherits it. `USD` renders `$12.50`.

