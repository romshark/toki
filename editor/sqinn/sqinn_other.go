//go:build !(darwin && arm64)

package sqinn

// No embedded binary for this platform.
// Build sqinn from https://github.com/cvilsmeier/sqinn with -DSQLITE_ENABLE_FTS5.
var gzipData []byte
