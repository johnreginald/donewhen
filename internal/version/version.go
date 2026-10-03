// Package version holds the build info. The Dockerfile sets both values with
// -ldflags; a plain `go build` or `go run` keeps the defaults.
package version

// Commit is the short git hash the binary was built from.
var Commit = "dev"

// BuiltAt is the build time (UTC, RFC 3339). It is empty for a plain build.
var BuiltAt = ""
