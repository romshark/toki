# CLI

```
toki <command> [flags]
```

| Command | What it does |
| --- | --- |
| [`generate`](#generate-and-lint) | Extracts texts from the source code and generates the localization bundle. |
| [`lint`](#generate-and-lint) | Checks the source code and the catalogs without writing any files. |
| [`edit`](#edit) | Opens the translation editor. |
| [`repair`](#repair) | Rewrites native-locale messages that disagree with their TIK. |
| [`version`](#version) | Prints version information. |
| `help` | Prints the usage of a command. |

`toki help` lists the commands. `toki help <command>` and `toki <command> -h` print the usage of a command.

## generate and lint

`toki generate` scans the Go module for TIKs, adds new messages to the catalogs, removes the messages that the source code no longer uses and generates the [bundle package](../2-guide/2-bundle-files.md). `toki lint` runs the same checks without writing any files.

| Flag | What it does |
| --- | --- |
| `-l locale` | The default locale of the texts in the source code, in non-und BCP 47. Required when the bundle doesn't exist yet. Once it exists, it must match the default locale of the bundle. |
| `-t locale` | A translation locale in non-und BCP 47. Creates a catalog for the locale if none exists. Repeat it for more locales. |
| `-m path` | The path to the Go module. Default: `.`. |
| `-b path` | The path to the bundle package relative to the module. Default: `tokibundle`. |
| `-require-complete` | Fails if a catalog has untranslated messages. |
| `-json` | Prints the result as JSON to stdout. |
| `-q` | Disables all console logging. |
| `-v` | Enables verbose console logging. |
| `-trimpath` | Trims source code paths. Default: `true`. |

## edit

`toki edit` opens the translation editor. It lists the TIKs of the module with their messages in every catalog, validates ICU messages while you type, writes the edits to the catalogs and regenerates the bundle.

| Flag | What it does |
| --- | --- |
| `-server addr` | Serves the editor over HTTP on `addr` instead of opening the desktop app. |
| `-b path` | The path to the bundle package. Default: `tokibundle`. |

The desktop app needs a toki built with cgo, which `go install` and `go run` use where a C compiler is available. Builds without cgo, such as the release binaries, need `-server`.

> [!TIP]
> The editor caches the scan of the module in `.toki/index.db`, together with the edits not yet written to the catalogs. Add `.toki/` to `.gitignore`.

## repair

`toki repair` rewrites the native-locale messages that disagree with their TIK from the TIK and regenerates the bundle. It doesn't add missing messages, which is what `toki generate` does.

| Flag | What it does |
| --- | --- |
| `-b path` | The path to the bundle package. Default: `tokibundle`. |

## version

`toki version` prints the version of Toki and the version of Go it was built with. Release builds add their commit and build date.

| Flag | What it does |
| --- | --- |
| `-full` | Also prints the build information with the module dependencies. |

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, also for a help request. |
| `1` | The command failed. |
| `2` | Invalid arguments or an unknown command. |
