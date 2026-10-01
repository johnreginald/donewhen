// Package db provides the Postgres connection pool and a minimal embedded
// SQL migration runner (no external tooling required).
package db

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/johnreginald/donewhen/internal/config"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Connect opens a pgx pool and pings it, retrying briefly so it tolerates a
// Postgres container that is still starting.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.ConnConfig.OnNotice = logMigrationNotice

	var pool *pgxpool.Pool
	deadline := time.Now().Add(30 * time.Second)
	for {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				return pool, nil
			} else {
				err = pingErr
				pool.Close()
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("connect postgres: %w", err)
		}
		time.Sleep(time.Second)
	}
}

// logMigrationNotice prints what a data-fixing migration reports with RAISE
// NOTICE. Only lines that start with "migration " are shown: Postgres also sends
// notices such as "relation already exists, skipping", which would be noise.
func logMigrationNotice(_ *pgconn.PgConn, n *pgconn.Notice) {
	if n != nil && strings.HasPrefix(n.Message, "migration ") {
		log.Print(n.Message)
	}
}

// migrateLockID is the Postgres advisory-lock key that serialises migration
// runs (the original project name in ASCII; the value is kept so old and new binaries share the lock).
const migrateLockID int64 = 0x5241454e494c

// DestructiveMarker, as the first line of a migration file, flags it as
// destructive: it drops or rewrites data, so a backup must exist first.
const DestructiveMarker = "-- donewhen:destructive"

// LegacyDestructiveMarker is the marker from before the rename to DoneWhen.
// Migrations are tracked by file name, so already-applied ones are unaffected;
// the gate still accepts the old marker.
const LegacyDestructiveMarker = "-- raenil:destructive"

// BackupConfirmedEnv names the env var that confirms a backup was taken. It
// holds a comma-separated list of the destructive migration names to allow.
const BackupConfirmedEnv = "DONEWHEN_BACKUP_CONFIRMED"

// BackupRequiredError is returned when a destructive migration is pending and
// no backup was confirmed for it.
type BackupRequiredError struct{ Names []string }

func (e *BackupRequiredError) Error() string {
	return fmt.Sprintf("refusing to migrate: destructive migration(s) pending: %s. "+
		"Back up the database first (make backup), then set %s=%s and start again, "+
		"or run `make migrate`, which takes the backup for you",
		strings.Join(e.Names, ", "), BackupConfirmedEnv, strings.Join(e.Names, ","))
}

// IsDestructive reports whether migration SQL carries the destructive marker
// on its first line.
func IsDestructive(sql string) bool {
	first, _, _ := strings.Cut(sql, "\n")
	first = strings.TrimSpace(first)
	return first == DestructiveMarker || first == LegacyDestructiveMarker
}

func migrationNames() ([]string, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// PendingDestructive lists destructive migrations not yet applied. On a
// database with no applied migrations (a fresh install) it returns none: there
// is no data to lose.
func PendingDestructive(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	var hasTable bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&hasTable); err != nil {
		return nil, err
	}
	if !hasTable {
		return nil, nil
	}
	applied, err := appliedSet(ctx, pool)
	if err != nil {
		return nil, err
	}
	return pendingDestructive(applied)
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func appliedSet(ctx context.Context, q querier) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func pendingDestructive(applied map[string]bool) ([]string, error) {
	if len(applied) == 0 {
		return nil, nil
	}
	names, err := migrationNames()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range names {
		if applied[n] {
			continue
		}
		b, err := migrationFS.ReadFile("migrations/" + n)
		if err != nil {
			return nil, err
		}
		if IsDestructive(string(b)) {
			out = append(out, n)
		}
	}
	return out, nil
}

// Migrate applies any embedded migrations not yet recorded in schema_migrations,
// reading the backup confirmation from DONEWHEN_BACKUP_CONFIRMED.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	return MigrateConfirmed(ctx, pool, config.Getenv(BackupConfirmedEnv))
}

// MigrateConfirmed is Migrate with an explicit backup confirmation (a
// comma-separated list of destructive migration names).
//
// A Postgres advisory lock on a dedicated connection serialises concurrent
// runners: the applied set is read only after the lock is held, so a second
// runner waits, then finds nothing to do. Files are applied in lexical order,
// each in its own transaction.
func MigrateConfirmed(ctx context.Context, pool *pgxpool.Pool, confirmed string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLockID); err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	defer func() {
		// Fresh context: ctx may be cancelled. If unlock fails, drop the
		// connection so the session end releases the lock.
		uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var ok bool
		if err := conn.QueryRow(uctx, `SELECT pg_advisory_unlock($1)`, migrateLockID).Scan(&ok); err != nil || !ok {
			_ = conn.Conn().Close(uctx)
		}
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedSet(ctx, conn)
	if err != nil {
		return err
	}

	// Gate destructive migrations before applying anything.
	pending, err := pendingDestructive(applied)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, n := range strings.Split(confirmed, ",") {
		allowed[strings.TrimSpace(n)] = true
	}
	var missing []string
	for _, n := range pending {
		if !allowed[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return &BackupRequiredError{Names: missing}
	}

	names, err := migrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		if applied[name] {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations(version) VALUES($1)`, name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		fmt.Printf("migrated: %s\n", name)
	}
	return nil
}
