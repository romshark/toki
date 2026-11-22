# Changelog

This file records the notable changes of each release in the format of
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The release
workflow publishes the section of a version as its GitHub release notes.
Releases up to v0.8.5 have their notes on
[GitHub Releases](https://github.com/romshark/toki/releases) only.

## [Unreleased]

### Added

#### All commands

- Print the usage, the flags, the log messages and the errors in English, German, French, Russian or Ukrainian, as the locale of the terminal sets: `LANGUAGE`, then the first of `LC_ALL`, `LC_MESSAGES` and `LANG`. The locale `C` and other languages get English. `-json` output stays English, as do the details that Go, the file system and the source scan add to an error.

#### `toki generate` and `toki lint`

- Write skills for AI coding agents to `.claude/skills/` and `.agents/skills/` in the module: `toki` for writing TIKs and `toki-translate` for translating the catalogs. Toki owns these files and rewrites them whenever they differ from what it writes. `-ai-skills=false` turns this off.

#### `toki edit`

- Add `toki edit`, a translation editor that replaces `toki webedit`. It lists the TIKs of the module with their messages in every catalog, filters them by status, locale and domain, searches them and shows where the source code uses each TIK. It validates ICU messages while you type, writes the edits to the catalogs and regenerates the bundle. When the bundle is out of date or corrupt, it offers to run `toki generate` or `toki repair`.
- Open the editor as a desktop app when toki is built with cgo, which `go install` uses where a C compiler is available, as do the macOS release binaries. `-server <address>` serves it over HTTP instead. Builds without cgo, such as the release binaries for Linux and Windows, require it. `-b` sets the bundle package path.
- Install toki on macOS with Homebrew: `brew install --cask romshark/tools/toki`.
- Cache the scan of the module in `.toki/index.db`, together with the edits not yet written to the catalogs. Add `.toki/` to `.gitignore`.
- Reorder the locales by dragging them in the sidebar. Every page shows them in that order, which `.toki/index.db` keeps. Until then, the default locale comes first.
- Collapse the Domains, Locales and Filter sections of the sidebar. The browser remembers which ones you closed.
- Pick the motion of the editor in the settings: Animated, Reduced Motion, or System, which follows the reduced motion setting of the OS.
- Show the editor in English, German, French, Russian or Ukrainian: the language you pick in the settings, else the language of the terminal in the desktop app and the language the browser prefers with `-server`.

#### `toki repair`

- Add `toki repair`, which rewrites the native-locale messages that disagree with their TIK from the TIK and regenerates the bundle. `-b` sets the bundle package path.

#### `toki help`

- Add `toki help`, which lists the commands, and `toki help <command>`, which describes a command and its flags. `toki -h` and `toki --help` print the list too. `toki` without a command prints it to stderr and exits with code 2.

#### `toki version`

- Accept `toki --version` and `toki -version`.

### Changed

#### `toki generate` and `toki lint`

- Fail with "bundle contains corrupt native-locale messages — run `toki repair`" when a native-locale message disagrees with its TIK, instead of generating the bundle from it. A message disagrees when its TIK fully determines it but the catalog has another text, or when its placeholder metadata doesn't match the TIK. To change such a message, change its TIK in the source code.
- Fill an empty native-locale message from its TIK instead of keeping it empty.
- Count blank messages as incomplete. `-require-complete` fails on catalogs with untranslated messages, and the completeness report counts them. A blank message used to count as complete once its catalog was written.
- Print the `-json` result to stdout instead of stderr.
- Print the usage for `-h` to stdout, headed by the name of the command and what it does instead of "Usage of toki:". Report an unknown flag in one line, without the usage.

#### `toki version`

- Print the Go version and platform instead of the build information with every dependency. `-full` adds the build information.

#### Requirements

- Require Go 1.27.4.

### Removed

#### `toki webedit`

- Remove `toki webedit`. Use `toki edit`.

### Fixed

#### `toki generate` and `toki lint`

- Generate the module `-m` points to. Loading its packages failed with "directory prefix ... does not contain main module", and the bundle package was created in the working directory.
- Find the bundle package on Windows instead of emptying its catalogs, and stop taking other packages whose path ends in the bundle's name for it.
- Reject a TIK used in multiple domains instead of writing a bundle that doesn't compile and breaks every later run.
- Judge `-require-complete` and the completeness report by the catalogs after adding new TIKs and removing deleted ones. A removed incomplete message kept failing the check, and a new blank one passed it.
- Stop panicking when the TIK removed from the source was the last one.
- Keep the existing Go files of the bundle when generating them fails. A formatting error left an empty `bundle_gen.go`.
- Return the error when writing a catalog fails partway instead of reporting success for a truncated `.arb` file.
- Report errors that stop the source scan in the `-json` output instead of panicking.

#### Generated bundle

Run `toki generate` to apply these.

- Render placeholders numbered 10 and above with their own argument. `{var10}` rendered argument 1.
- Pick the option of a `selectordinal` by the ordinal plural rules. English rendered 2 as "2th" instead of "2nd".
- Support exact `=n` plural options and plural offsets. `=n` options were ignored, and the plural category didn't subtract the offset.
- Unescape apostrophes in catalog messages like ICU does. "He's here" rendered as "Hes here".
- Generate valid Go for TIKs that contain a backtick. Generation failed with a formatting error.
- Render a translation that selects on the gender of a `{name}`, as `{var0_gender, select, ...}`, or on its value. The bundle panicked with "interface conversion: interface {} is tokibundle.String, not string".

#### `toki version`

- Print `Toki (devel)` instead of `Toki v(devel)` for `go run` in a local checkout.

[Unreleased]: https://github.com/romshark/toki/compare/v0.8.5...HEAD
