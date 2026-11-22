---
name: Toki
repository: https://github.com/romshark/toki
address: https://romshark.github.io/toki/
suggested: true
---

# Introduction

Toki is an i18n framework for Go and a Textual Internationalization Key ([TIK](https://github.com/romshark/tik)) processor implementation.

`toki generate` parses the source code, lints it reporting any misuse or errors, extracts localized texts and generates a localization bundle.

[app-resource-bundle (.arb)](https://github.com/google/app-resource-bundle) files are used as intermediate translation storage for [ICU messages](https://unicode-org.github.io/icu/userguide/format_parse/messages/) generated from TIKs extracted from the source code.

`toki edit` opens a desktop editor app for comfortable manual translation editing. Pass `-server <addr>` to run it as a plain HTTP server instead.

## Next steps

Follow the [quick start](1-get-started/1-quick-start.md) to internationalize a Go program, then [add languages](1-get-started/2-add-languages.md) to it.
