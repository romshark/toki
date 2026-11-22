# Continuous integration

To make sure you never deploy broken or unfinished localizations, add Toki to your CI/CD pipeline:

```sh
go run github.com/romshark/toki@latest lint -require-complete
```

You may also run `toki generate -require-complete` and check with `git diff` that your generated bundle package is up to date.
