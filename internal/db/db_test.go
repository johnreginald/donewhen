package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// scratchPool creates a throwaway database, returns a pool on it and drops it
// at the end. Needs RAENIL_TEST_DATABASE_URL (any database on a test server);
// otherwise the test is skipped. It never touches the shared database itself.
func scratchPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("RAENIL_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set RAENIL_TEST_DATABASE_URL to run db tests")
	}
	ctx := context.Background()
	admin, err := Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	name := fmt.Sprintf("raenil_scratch_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		admin.Close()
		t.Fatalf("create scratch db: %v", err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := Connect(ctx, u.String())
	if err != nil {
		t.Fatalf("connect scratch: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`); err != nil {
			t.Errorf("drop scratch db: %v", err)
		}
		admin.Close()
	})
	return pool
}

func appliedCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigrateFreshThenNoop(t *testing.T) {
	pool := scratchPool(t)
	ctx := context.Background()
	// Fresh DB: destructive migrations apply without a backup (no data).
	if err := MigrateConfirmed(ctx, pool, ""); err != nil {
		t.Fatalf("fresh migrate: %v", err)
	}
	names, _ := migrationNames()
	if got := appliedCount(t, pool); got != len(names) {
		t.Fatalf("applied %d, want %d", got, len(names))
	}
	if err := MigrateConfirmed(ctx, pool, ""); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if got := appliedCount(t, pool); got != len(names) {
		t.Fatalf("after no-op applied %d, want %d", got, len(names))
	}
}

func TestMigrateConcurrentRunsApplyOnce(t *testing.T) {
	pool := scratchPool(t)
	ctx := context.Background()
	names, _ := migrationNames()

	const runners = 4
	errs := make([]error, runners)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < runners; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = MigrateConfirmed(ctx, pool, "")
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("runner %d: %v", i, err)
		}
	}
	if got := appliedCount(t, pool); got != len(names) {
		t.Fatalf("applied %d rows, want %d", got, len(names))
	}
}

// The lock is held on a connection of its own: while another session holds it,
// Migrate blocks, and proceeds the moment it is released.
func TestMigrateWaitsForLock(t *testing.T) {
	pool := scratchPool(t)
	ctx := context.Background()

	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLockID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- MigrateConfirmed(ctx, pool, "") }()
	select {
	case err := <-done:
		t.Fatalf("Migrate returned while lock was held: %v", err)
	case <-time.After(700 * time.Millisecond):
	}
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_unlock($1)`, migrateLockID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("migrate after unlock: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Migrate did not proceed after unlock")
	}
}

// The lock is released on error too: a failed run must not wedge the next.
func TestMigrateReleasesLockOnError(t *testing.T) {
	pool := scratchPool(t)
	ctx := context.Background()
	if err := MigrateConfirmed(ctx, pool, ""); err != nil {
		t.Fatal(err)
	}
	// Mark an old destructive migration unapplied so the next run is refused.
	if _, err := pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version='0036_plain_tracker.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := MigrateConfirmed(ctx, pool, ""); err == nil {
		t.Fatal("expected refusal")
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, migrateLockID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("advisory lock still held after failed Migrate")
	}
	_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock_all()`)
}

func TestDestructiveGate(t *testing.T) {
	pool := scratchPool(t)
	ctx := context.Background()
	if err := MigrateConfirmed(ctx, pool, ""); err != nil {
		t.Fatal(err)
	}
	// Existing deployment: 0036 not yet applied, everything else is.
	const name = "0036_plain_tracker.sql"
	if _, err := pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version=$1`, name); err != nil {
		t.Fatal(err)
	}

	pending, err := PendingDestructive(ctx, pool)
	if err != nil || len(pending) != 1 || pending[0] != name {
		t.Fatalf("pending = %v, err %v", pending, err)
	}

	// No confirmation -> refused, error names the migration, nothing applied.
	err = MigrateConfirmed(ctx, pool, "")
	var bre *BackupRequiredError
	if !errors.As(err, &bre) || !strings.Contains(err.Error(), name) {
		t.Fatalf("want BackupRequiredError naming %s, got %v", name, err)
	}
	// Confirmation for a different migration does not count.
	if err := MigrateConfirmed(ctx, pool, "0001_init.sql"); err == nil {
		t.Fatal("wrong confirmation accepted")
	}
	if got := appliedCount(t, pool); got != mustLen(t)-1 {
		t.Fatalf("migration applied despite refusal (count %d)", got)
	}

	// Confirmed by name (among others) -> applies.
	if err := MigrateConfirmed(ctx, pool, "foo.sql, "+name); err != nil {
		t.Fatalf("confirmed migrate: %v", err)
	}
	if got := appliedCount(t, pool); got != mustLen(t) {
		t.Fatalf("count %d after confirmed run", got)
	}
}

func mustLen(t *testing.T) int {
	t.Helper()
	names, err := migrationNames()
	if err != nil {
		t.Fatal(err)
	}
	return len(names)
}

func TestIsDestructive(t *testing.T) {
	cases := map[string]bool{
		"-- raenil:destructive\nDROP TABLE x;":    true,
		"-- raenil:destructive  \r\nDROP TABLE x": true,
		"-- note\n-- raenil:destructive\nSELECT":  false,
		"SELECT 1;":                               false,
		"":                                        false,
	}
	for in, want := range cases {
		if got := IsDestructive(in); got != want {
			t.Errorf("IsDestructive(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestOnly0036IsMarked(t *testing.T) {
	names, _ := migrationNames()
	for _, n := range names {
		b, _ := migrationFS.ReadFile("migrations/" + n)
		if got, want := IsDestructive(string(b)), n == "0036_plain_tracker.sql"; got != want {
			t.Errorf("%s destructive = %v, want %v", n, got, want)
		}
	}
}
