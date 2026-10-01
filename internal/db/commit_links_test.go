package db

import (
	"context"
	"testing"
)

const commitLinksMigration = "0040_commit_links_unique.sql"

// PP-187: the migration, run over a table that holds duplicate and mixed-case
// links, keeps the earliest row per (issue, sha) and lowercases the shas.
func TestCommitLinksMigrationDedupes(t *testing.T) {
	pool := scratchPool(t)
	ctx := context.Background()
	if err := MigrateConfirmed(ctx, pool, ""); err != nil {
		t.Fatal(err)
	}
	one := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	ws := one(`INSERT INTO workspaces (slug, name, key_prefix) VALUES ('cm-a','A','CMA') RETURNING id`)
	st := one(`INSERT INTO workflow_states (name, category, workspace_id) VALUES ('Backlog','backlog',$1) RETURNING id`, ws)
	issue := func(n int, key string) string {
		return one(`INSERT INTO issues (workspace_id, number, key, title, description_md, state_id, priority)
			VALUES ($1,$2,$3,'t','',$4,3) RETURNING id`, ws, n, key, st)
	}
	a, b := issue(1, "CMA-1"), issue(2, "CMA-2")

	// Back to the pre-migration shape: no constraint, no index.
	exec(`ALTER TABLE issue_commits DROP CONSTRAINT issue_commits_issue_sha_key`)
	exec(`DROP INDEX issue_commits_sha_idx`)

	link := func(issueID, sha, msg, at string) {
		exec(`INSERT INTO issue_commits (issue_id, sha, message, created_at) VALUES ($1,$2,$3,$4::timestamptz)`,
			issueID, sha, msg, at)
	}
	link(a, "abc1234", "first", "2026-01-01T00:00:00Z")
	link(a, "abc1234", "dup", "2026-01-02T00:00:00Z")
	link(a, "ABC1234", "dup, other case", "2026-01-03T00:00:00Z")
	link(a, "def5678", "other sha", "2026-01-01T00:00:00Z")
	link(b, "abc1234", "same sha, other issue", "2026-01-01T00:00:00Z")

	sqlBytes, err := migrationFS.ReadFile("migrations/" + commitLinksMigration)
	if err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 2; run++ { // twice: it must be safe to re-run
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}

	rows, err := pool.Query(ctx, `SELECT issue_id, sha, message FROM issue_commits ORDER BY message`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{} // message -> issue/sha
	for rows.Next() {
		var iss, sha, msg string
		if err := rows.Scan(&iss, &sha, &msg); err != nil {
			t.Fatal(err)
		}
		got[msg] = iss + "/" + sha
	}
	want := map[string]string{
		"first":                 a + "/abc1234",
		"other sha":             a + "/def5678",
		"same sha, other issue": b + "/abc1234",
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("row %q = %q, want %q", k, got[k], v)
		}
	}

	// The constraint is back: a duplicate is refused.
	if _, err := pool.Exec(ctx, `INSERT INTO issue_commits (issue_id, sha) VALUES ($1,'abc1234')`, a); err == nil {
		t.Fatal("duplicate (issue, sha) accepted after the migration")
	}
}

// The migration deletes rows, so it must carry the destructive marker.
func TestCommitLinksMigrationIsGated(t *testing.T) {
	b, err := migrationFS.ReadFile("migrations/" + commitLinksMigration)
	if err != nil {
		t.Fatal(err)
	}
	if !IsDestructive(string(b)) {
		t.Fatalf("%s deletes rows but is not marked destructive", commitLinksMigration)
	}
}
