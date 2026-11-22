---
name: toki
description: >-
  Write user-facing text in Go with Toki: TIK syntax, placeholders and the
  argument types they take, translator comments, contexts, domains, the
  {{.BundlePkg}} package API and running toki generate. Use when adding,
  changing or removing text that users read, or when toki generate or toki
  lint reports an error.
---

{{template "header"}}

# Toki

This Go module localizes its text with Toki. Each text that users read is a TIK, a Textual Internationalization Key: the text itself, written in the default locale, which is also the key of its translations. `toki generate` finds every TIK in the source code, adds new ones to the catalogs, removes those the code no longer uses and compiles the catalogs into the bundle package.

| | |
| --- | --- |
| Bundle package | `{{.BundleDir}}/`, imported as `{{.BundleImportPath}}` |
| Default locale | `{{.DefaultLocale}}`, the language of the TIKs |
| Catalogs | `{{.BundleDir}}/catalog_<locale>.arb`, `{{.NativeCatalog}}` for `{{.DefaultLocale}}` |

To translate the catalogs, read the `toki-translate` skill.

## Run toki

{{template "run" .}}

Run `toki generate` after every change to a TIK. It reports each error with its position and exits with status 1. Its last lines give the completeness of each catalog.

## Read a localized text

A `{{.BundlePkg}}.Reader` renders the texts of 1 locale. Pass the reader the code already has down to where it renders text. Get one per user or request, not per text:

```go
// The best catalog for the locales, in order of preference.
tags, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
reader, _ := {{.BundlePkg}}.Match(tags...)

reader = {{.BundlePkg}}.Default() // The catalog of the default locale.
```

`language` is `golang.org/x/text/language`. `{{.BundlePkg}}.Locales()` lists the locales of the bundle.

Pass a TIK and its arguments to `String`, or to `Write`, which writes to an `io.Writer`:

```go
// Greeting on the dashboard after sign-in.
title := reader.String(`Welcome back, {name}!`, user)

// Status line under the inbox.
_, err := reader.Write(w, `You have {# unread messages}, last synced at {time-short}.`, unread, syncedAt)
```

## Write a TIK

- Write the TIK as a string constant: a literal or a `const`. Never build it from variables with `+` or `fmt.Sprintf`. Pass the values as arguments to placeholders.
- Write a whole sentence as 1 TIK. Don't join localized pieces: word order differs between languages.
- Write the text in the plural form `other`, the one English uses for 5: `{# files}`, never `{# file(s)}`.
- `{`, `}` and `\` are syntax. Write `\{`, `\}` and `\\` for the characters themselves. Raw string literals in backticks keep these escapes readable.
- Localize what users read. Log messages and errors for developers usually stay plain strings: follow what the surrounding code does.

Full syntax: [TIK cheatsheet](https://romshark.github.io/tik/cheatsheet/).

## Placeholders

Each placeholder takes 1 argument, in the order the placeholders appear in the TIK. `toki generate` checks the number and the types of the arguments.

| Placeholder | Argument | Example TIK |
| --- | --- | --- |
| `{text}` | a `string`, or a type whose underlying type is `string` | `Folder {text} is empty.` |
| `{name}` | `{{.BundlePkg}}.String{Value: "Ada", Gender: {{.BundlePkg}}.GenderFemale}` | `{name} joined the team.` |
| `{integer}` | any integer type | `Score: {integer}` |
| `{number}` | `float32` or `float64` | `Ratio: {number}` |
| `{# ...}` | any integer or float type | `{# files} copied.` |
| `{ordinal}` | any integer or float type | `You're {ordinal} in line.` |
| `{date-full}`, `{date-long}`, `{date-medium}`, `{date-short}` | `time.Time` | `Due on {date-short}.` |
| `{time-full}`, `{time-long}`, `{time-medium}`, `{time-short}` | `time.Time` | `Sent at {time-short}.` |
| `{currency}` | `{{.BundlePkg}}.Currency{Amount: 9.99, Type: currency.EUR}` | `Total: {currency}` |

- Use `{name}` for people and other things whose grammatical gender changes the sentence in some languages, and `{text}` for any other string. The genders are `{{.BundlePkg}}.GenderNeutral`, `GenderMale` and `GenderFemale`.
- `{# ...}` is a cardinal plural. `#` renders the number and the rest is text, which may hold other placeholders: `{# files in {text}}`. A plural can't hold another plural, can't start with a placeholder and can't end with a space. `{#}` renders the number alone.
- An untyped constant such as `3` is an `int`. Pass `3.0` or `float64(n)` to `{number}`.
- `currency` is `github.com/go-playground/locales/currency`.
- Pass the arguments one by one. `args...` works only with a slice literal written in the call.

## Contexts

A text that reads the same but means different things needs a context in brackets at its start:

```go
reader.String(`[sort order] Order`)
reader.String(`[purchase button] Order`)
```

Users don't see the context. It can't contain `{`, `}`, `[`, `]` or `\`, and a space must follow its `]`. Identical TIKs share 1 message and 1 translation.

## Translator comments

The `//` lines directly above the line where the call starts become the description of the message, which translators read. Write one whenever the TIK leaves room for doubt: where the text appears, what a placeholder holds, a length limit.

```go
// Button that starts a free trial. Keep it under 20 characters.
label := reader.String(`Start`)
```

A blank line between the comment and the call drops the comment. Changing a comment keeps the translations.

## Changing a TIK drops its translations

The ID of a message is a hash of its TIK and its domain. Changing a TIK, even to fix a typo, makes `toki generate` remove the old message with its translations from every catalog and add the new one, blank in the catalogs of other locales. Tell the user which translations a change discards. Where a message is blank, readers get its text in `{{.DefaultLocale}}`.

## Domains

A `.tokidomain.yml` file names the directory tree it is in. `toki generate` creates the one in the module root, and others in subdirectories create sub-domains. The `description` in a domain file tells translators the tone and terms of its texts.

Adding, renaming or removing a domain file, or moving a TIK into another domain, gives its messages new IDs and drops their translations. Don't do it unless the user asks. A TIK used in 2 domains is an error: give one of them a context.

## After toki generate

- New messages in `{{.NativeCatalog}}` hold the ICU message that Toki writes from the TIK. Where `{{.DefaultLocale}}` needs a plural form that a TIK can't express, as English `one` for `{# files}`, the message is incomplete. Complete it as the `toki-translate` skill describes, then run `toki generate` again.
- Toki scans only the files the current build configuration includes. A TIK in a file that build constraints leave out, as `//go:build windows` on macOS, is unseen, and `toki generate` removes its messages. `GOOS`, `GOARCH` and `GOFLAGS=-tags=...` change what it scans.

## Don't

- Never edit `{{.BundleDir}}/*_gen.go` or any other file with a `DO NOT EDIT` header. `toki generate` overwrites them.
- Never change message IDs or the `@` metadata in the `.arb` files.
- Never edit a message in `{{.NativeCatalog}}` that its TIK fully determines. Change the TIK in the source code instead.

## Errors

| Error | Fix |
| --- | --- |
| `TIK: not a constant`, `TIK: not a string constant` | Pass a string literal or a string `const`. |
| `TIK: arg N must be ... but received: T` | Convert the argument, or use the placeholder for its type. |
| `TIK: missing argument N for placeholder` | Add the argument. |
| `TIK: arg X doesn't match any TIK placeholder` | Remove the argument, or add its placeholder to the TIK. |
| `TIK: can't unpack composite literal` | Pass the arguments one by one. |
| `TIK used in multiple domains` | Give one of the TIKs a context. |
| `TIK collision` | 2 TIKs have the same hash. Give one of them a context. |
| `bundle contains corrupt native-locale messages` | Undo the edit of `{{.NativeCatalog}}`, or run `toki repair{{.Flags}}`, which rewrites such messages from their TIKs. |
