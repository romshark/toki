# Continuous integration

To make sure you never deploy broken or unfinished localizations, add Toki to your CI/CD pipeline:

```sh
go run github.com/romshark/toki@latest lint -require-complete
```

`lint` fails on errors in the source code and the catalogs, and `-require-complete` fails on messages that aren't fully translated.

## Check that the bundle is up to date

`lint` writes no files and doesn't fail when the catalogs or the bundle lag behind the source code. To catch that too, generate the bundle and fail if it changed:

```sh
go run github.com/romshark/toki@latest generate -require-complete
git diff --exit-code
```

> [!IMPORTANT]
> The bundle records the version of Toki that generated it. Run the same version in CI as locally, as `github.com/romshark/toki@v1.2.3` in place of `@latest`. Another version changes the generated files and fails the check.
