// Package testdb gives database tests one place to find the throwaway
// Postgres. Without DONEWHEN_TEST_DATABASE_URL a test skips, so a bare
// `go test ./...` stays usable. With DONEWHEN_TEST_STRICT set (which
// `make test` does) a missing database is a failure, so a run that tested
// nothing cannot pass.
package testdb

import (
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
)

// DSN returns the test database URL, skipping (or failing in strict mode)
// when it is not set.
func DSN(t testing.TB) string {
	t.Helper()
	dsn := config.Getenv("DONEWHEN_TEST_DATABASE_URL")
	if dsn != "" {
		return dsn
	}
	if config.Getenv("DONEWHEN_TEST_STRICT") != "" {
		t.Fatal("DONEWHEN_TEST_DATABASE_URL is not set but DONEWHEN_TEST_STRICT is: run `make test`")
	}
	t.Skip("set DONEWHEN_TEST_DATABASE_URL to run database tests")
	return ""
}
