---
name: changelog
description: Write CHANGELOG.md entries and release a version. Use when adding a changelog entry or releasing.
---

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The release workflow publishes each tag's section as its GitHub release notes through `internal/tools/release-notes` and fails when the section or its compare link is missing. `TestChangelogReleases` validates the version headings.

- Add user-visible changes of the CLI, the generated bundle and the editor under `## [Unreleased]` in the same commit. Omit `docs`, `test`, `chore` and `ci` changes, and fixes of changes that weren't released yet.
- Sort entries into `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed` and `Security`.
- Within a section, group entries under `####` subheadings by where the change applies, in this order: `` `toki generate` and `toki lint` `` for checks, catalogs and reports; `Generated bundle`, opened by the line "Run `toki generate` to apply these."; a subheading per other command, such as `` `toki edit` ``; `Requirements` for the Go version and dependencies. Leave out a subheading with no entries.
- Don't repeat in an entry what its subheading says: no "in `toki generate`" and no closing "Run `toki generate`."
- Use imperative entries like commit titles: `Reject TIKs used in multiple domains`. Say enough for users to determine whether they are affected. A fix states what went wrong before, as in "English rendered 2 as "2th" instead of "2nd"."
- A breaking change gives the steps to migrate.
- To release, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`, insert an empty `## [Unreleased]`, update the compare links at the end of the file and tag `vX.Y.Z`.
