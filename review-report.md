# Repository bug review

Reviewed commit: `8db0637` (`chore: Upgrade dependencies`)

The findings below are ordered by severity. Several generator failures were verified with temporary focused tests; those tests were removed after the review.

## High severity

### 1. Editor builds use stale ICU tokens after changing a translation

Locations: `internal/cli/generate.go:637-664`, `internal/gengo/gengo.go:189-191`, `internal/gengo/iowriter.go:11-14`

`ApplyChangesAndBuild` replaces `msg.ICUMessage`, but it does not parse the new value or replace `msg.ICUMessageTokens`. It then generates the bundle from that inconsistent in-memory scan. As a result, a translation whose structure changes is rendered using the old token boundaries. This can silently emit the new ICU syntax as literal text, truncate it, or cause an out-of-range slice panic.

Reproduced by changing a blank German translation to `Hallo {var0}` through `ApplyChangesAndBuild`: the generated function returned the literal string `Hallo {var0}` instead of substituting the argument.

The edited message must be reparsed and validated before mutating the scan or generating output.

### 2. Generated placeholders numbered 10 and above reference the wrong argument

Location: `internal/gengo/gengo.go:258-276`

The placeholder parser loops over `s[endIndex:]` but checks `s[i]`, where `i` is relative to the sliced string. Because it starts testing at the beginning of the original string again, it never advances through all digits of a multi-digit name. `var10`, for example, is parsed as argument 1.

Reproduced with placeholders `var0` through `var10`: the generated output was `0|1|2|3|4|5|6|7|8|9|1`.

### 3. `selectordinal` uses cardinal plural rules

Locations: `internal/gengo/iowriter.go:53-56`, `internal/gengo/iowriter.go:203-227`

`TokenTypeSelectOrdinal` calls `writePlural(false)`, selecting the cardinal rule getter. Ordinal messages therefore choose the wrong branch. In English, a value of 2 can select `other` instead of ordinal category `two`, producing output such as `2th` instead of `2nd`.

### 4. Plural exact-number branches and offsets are generated incorrectly

Locations: `internal/gengo/iowriter.go:192-227`, `internal/gengo/iowriter.go:262-289`

The plural generator:

- does not emit `=n` exact-number options at all;
- calculates the plural category from the original argument instead of `argument - offset`; and
- applies the offset only when rendering `#`.

Valid ICU plural messages using exact matches or offsets therefore silently choose an incorrect branch.

### 5. Removing the final TIK can panic generation

Location: `internal/cli/generate.go:268-285`

When an ID from the native catalog is no longer present in the source scan, `TextIndexByID.Get` returns `ok == false`. The removal path nevertheless calls `scan.Texts.At(index)`. The zero index either refers to an unrelated active TIK or, when no TIKs remain, panics.

Reproduced by generating a project with one TIK, deleting that TIK from the source, and generating again; the second generation panicked.

### 6. The editor database cache is not invalidated by Go source changes and loses scan state

Locations: `editor/app/app.go:278-323`, `editor/app/app.go:372-382`, `editor/app/app.go:505-522`, `editor/app/app.go:619-680`, `editor/indexdb/indexdb.go:33-39`, `editor/indexdb/indexdb.go:81-100`

The fast-path cache is validated using ARB checksums, schema version, and Toki version, but not a fingerprint of the Go source. Adding, changing, or removing TIKs without touching the catalogs can therefore reopen stale data.

The persisted representation also omits source occurrences and the missing-message count. A slow scan with missing native entries can populate the database, but the next startup restores `numMissing` as zero and no longer triggers the missing-message warning/regeneration path. Source links also disappear on the cached startup.

The cache key needs a source/build fingerprint, and every state item used by editor behavior must either be persisted or recomputed.

### 7. Backticks in valid TIK strings produce invalid Go and erase the existing bundle

Locations: `internal/gengo/gengo.go:127-133`, `internal/cli/generate.go:679-695`

TIK text is embedded in a raw backtick string in generated Go without escaping or choosing another literal form. A valid source TIK such as `Use `code`` consequently creates invalid generated source.

The output file is opened with `O_TRUNC` before `format.Source` succeeds, so the formatting error also leaves the previous `tokibundle/bundle_gen.go` empty. This turns an input-specific generation error into destructive loss of the last valid bundle.

Both conditions were reproduced. Generation should format into memory (or a temporary file) and atomically replace the destination only after success.

### 8. The editor requires a public CDN despite its offline requirement

Locations: `editor/datapagesgen/app_gen.go:43-44`, `editor/datapagesgen/app_gen.go:293-302`, `editor/datapagesgen/app_gen.go:451-453`, `editor/editor.go:100-103`

Every generated page loads Datastar from jsDelivr by default. Editor setup supplies embedded assets and middleware but does not override Datastar with an embedded local asset, and the static bundle does not otherwise provide it. With no network access, Datastar actions and SSE-driven UI behavior cannot initialize.

This contradicts the repository's stated requirement that the editor remain fully offline-capable.

## Medium severity

### 9. JSON error reporting panics when source scanning fails

Locations: `internal/cli/result.go:49-64`, `internal/cli/result.go:94-103`, `internal/codeparse/codeparse.go:172-179`

Package-loading failures return without a scan, but JSON conversion unconditionally dereferences `r.Scan`. Calling `Print` with JSON output after an invalid-source/package-load error therefore panics instead of returning a machine-readable error.

This was reproduced using invalid Go source with `-json`; `result.Scan` was nil and `result.Print()` panicked.

### 10. `-m` cannot generate a module outside the current working module

Locations: `internal/config/config.go:41`, `internal/cli/generate.go:66-92`, `internal/cli/generate.go:121-123`, `internal/cli/generate.go:177-185`, `internal/codeparse/codeparse.go:158-172`

The option is documented as a path to a Go module, but package loading treats it as a pattern relative to the current module instead of setting the loader's working directory. Most generated paths also ignore `ModPath` and are constructed from `BundlePkgPath` relative to the current process directory.

Reproduced by targeting another module with an absolute `-m` path: loading failed with “directory prefix ... does not contain main module”, while a `tokibundle` directory was created under the caller's current directory.

### 11. `sync.Map` iterators release their locks before iteration begins

Location: `internal/sync/map.go:64-85`

`Seq` and `SeqRead` acquire a lock and defer its release in the outer function, then return an iterator closure. The defer runs as soon as the closure is returned, so the actual map iteration occurs without the advertised lock. Concurrent writes can race with iteration or trigger a fatal concurrent-map iteration/write error.

The lock must be acquired inside the returned iterator closure, as the slice implementation already does.

### 12. Search results remain stale after an editor update

Locations: `editor/app/app.go:550-563`, `editor/app/app.go:889-892`, `editor/indexdb/indexdb.go:347-358`

The FTS table is populated initially, but `POSTSet` calls an update method that changes only the `message` table. It never updates `search_index`. Searches therefore continue matching the old text and cannot find newly entered text until the index is rebuilt.

### 13. Search mode ignores active filters and can render an empty clamped page

Locations: `editor/app/app.go:1584-1612`

Any non-empty query switches to `buildSearch`, which receives locale visibility but not the selected domain or translation-status filter. Search results thus include items that the current sidebar filters claim to exclude.

The database query also runs using the requested page offset before the page number is clamped against the returned total. If a user requests a page beyond the last page, the code clamps the displayed page number but does not rerun the query, producing an empty page instead of the actual last page.

### 14. Completeness counters become stale while catalogs are synchronized

Locations: `internal/cli/generate.go:241-285`, `internal/cli/generate.go:332-338`, `internal/cli/generate.go:752-759`

Adding a missing entry with a blank translation does not increment `MessagesIncomplete`, and removing an obsolete blank entry does not decrement it. Later `--require-complete` and completeness reporting trust this counter, so generation can succeed while new translations are blank or continue failing after the last incomplete obsolete item was removed.

Completeness should be recomputed from the final catalog state, or maintained on every mutation.

### 15. ARB encoding suppresses most writer errors

Location: `internal/arb/arb.go:422-456`

Nearly every `Write`/`Fprintf` result is ignored; only the final closing-brace write is returned. A writer can fail partway through and later accept the final write, causing `Encode` to return nil for truncated output. Callers may then treat a corrupted catalog as successfully written.

Every write error should be propagated immediately.

### 16. Repeated editor builds leak open files

Locations: `internal/codeparse/codeparse.go:288-297`, `internal/cli/generate.go:682-720`

Opened ARB inputs and generated bundle/catalog outputs are not closed. This is particularly harmful in the long-lived editor process, where each build leaks another group of descriptors and can eventually fail with “too many open files” or leave buffered/write errors undetected.

### 17. Server-side updates do not enforce read-only messages

Location: `editor/app/app.go:834-892`

`POSTSet` resolves and updates a message without checking `ICUMessage.IsReadOnly`. The client UI may discourage editing, but a crafted or stale request can still change a read-only/native value and persist it to the database before generation.

Read-only status must be validated at the mutation boundary, not only represented in the UI.

## Low severity

### 18. Domain signal encoding corrupts underscores and creates collisions

Locations: `editor/app/app.go:1366-1376`, `editor/app/app.go:1573-1576`, `internal/codeparse/domain.go:98-101`

Domain names are encoded by replacing dots with underscores and decoded by replacing every underscore with a dot. Since underscores are valid in domain names, `foo_bar` is interpreted as `foo.bar`, and the two distinct domains collide in signal names and filtering state.

A reversible encoding or opaque domain ID is required.

## Validation notes

- `go vet ./...` passed.
- An initial `go test ./...` run passed. After the dependency-only HEAD update shown above, a full rerun could not reliably execute the root integration package in the sandbox because its temporary nested modules could not load cached imports; all non-root packages passed.
- Focused temporary tests reproduced findings 1, 2, 5, 7, 9, and 10.
- The frontend production build could not run because local `node_modules`/the esbuild binary are not installed.
- The locally installed `datapages` generator is a different version from the module dependency, so its reported generated-file differences were not treated as repository defects.
