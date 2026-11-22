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
| `-t locale` | A translation locale in non-und BCP 47. Creates a catalog for the locale if none exists. Repeat it for more locales. `lint` ignores it. |
| `-m path` | The path to the Go module. Default: `.`. |
| `-b path` | The path to the bundle package relative to the module. Default: `tokibundle`. |
| `-require-complete` | Fails if a catalog has blank messages or messages that lack plural or select options. |
| `-ai-skills` | Writes the [skills for AI coding agents](../2-guide/3-ai-agents.md). Default: `true`. Turn it off with `-ai-skills=false`. `lint` ignores it. |
| `-json` | Prints the result as JSON to stdout, and the logs as JSON to stderr. |
| `-q` | Logs only errors. |
| `-v` | Logs more details, among them the options that each message lacks. |
| `-trimpath` | Logs the positions of TIKs relative to the module. Default: `true`. Turn it off with `-trimpath=false`. |

Toki scans the packages that the current build configuration includes. TIKs in files that build constraints leave out, as a file with `//go:build windows` on macOS, stay unseen, and `toki generate` removes their messages. `GOOS`, `GOARCH` and `GOFLAGS=-tags=…` change what Toki scans.

## edit

`toki edit` opens the translation editor. It lists the TIKs of the module with their messages in every catalog, validates ICU messages while you type, writes the edits to the catalogs and regenerates the bundle. Run it in the root of the module, after `toki generate` created the bundle.

| Flag | What it does |
| --- | --- |
| `-server addr` | Serves the editor over HTTP on `addr` instead of opening the desktop app. |
| `-b path` | The path to the bundle package relative to the current folder. Default: `tokibundle`. |

The desktop app needs a toki built with cgo, which `go install` and `go run` use where a C compiler is available. Builds without cgo, such as the release binaries for Linux and Windows, need `-server`.

> [!WARNING]
> The editor has no authentication: anyone who reaches `addr` can change your catalogs. Serve it on a loopback address, as `127.0.0.1:8080`.

> [!TIP]
> The editor caches the scan of the module in `.toki/index.db`, together with the edits not yet written to the catalogs. Add `.toki/` to `.gitignore`.

## repair

`toki repair` rewrites the native-locale messages that disagree with their TIK from the TIK and regenerates the bundle. It doesn't add missing messages, which is what `toki generate` does. Run it in the root of the module.

| Flag | What it does |
| --- | --- |
| `-b path` | The path to the bundle package relative to the current folder. Default: `tokibundle`. |

## version

`toki version`, or `toki --version`, prints the version of Toki and the version and platform of Go it was built with. It adds the commit and its date when the build records them, as release builds and builds in a Git checkout do.

| Flag | What it does |
| --- | --- |
| `-full` | Also prints the build information with the module dependencies. |

## Language

Toki prints its usage, its log messages and its errors in English, German, French, Russian or Ukrainian, following the locale of the terminal as GNU gettext does: the first of `LC_ALL`, `LC_MESSAGES` and `LANG` that is set, preceded by the languages that `LANGUAGE` lists. The locale `C` and the languages that Toki has no translation for get English. `-json` output is always English.

```sh
LANG=de_DE.UTF-8 toki help
```

The editor that `toki edit` opens shows the language you pick in its settings. Until you pick one, the desktop app follows the terminal, and `-server` follows the language the browser prefers.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, also for a help request. |
| `1` | The command failed. |
| `2` | Invalid arguments or an unknown command. |
