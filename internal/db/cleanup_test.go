package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

const cleanupMigration = "0039_security_cleanup.sql"

// PP-236 parts 3 and 4: the clean-up migration, run over fixture rows, deletes
// only the unsafe push subscriptions and clears only the foreign references.
func TestSecurityCleanupMigration(t *testing.T) {
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

	var uid = one(`INSERT INTO users (email, password_hash) VALUES ('cleanup@example.test', 'x') RETURNING id`)

	// --- push subscriptions ---
	endpoints := map[string]bool{ // endpoint -> kept
		"https://fcm.googleapis.com/fcm/send/abc":      true,
		"https://updates.push.services.mozilla.com/x":  true,
		"https://8.8.8.8/push":                         true,
		"https://[2606:4700:4700::1111]/push":          true,
		"https://some-host.example.com:8443/push":      true,
		"http://fcm.googleapis.com/fcm/send/plain":     false, // not https
		"ftp://example.com/x":                          false,
		"not a url at all":                             false,
		"https://127.0.0.1/push":                       false,
		"https://10.1.2.3:8443/push":                   false,
		"https://192.168.0.7/push":                     false,
		"https://172.16.5.5/push":                      false,
		"https://169.254.169.254/latest/meta-data":     false,
		"https://100.64.0.1/push":                      false,
		"https://user:pw@10.0.0.9/push":                false,
		"https://[::1]/push":                           false,
		"https://[fd00::1]/push":                       false,
		"https://[::ffff:10.0.0.1]/push":               false,
		"https://localhost/push":                       false,
		"https://printer.local/push":                   false,
		"https://db.internal/push":                     false,
		"https://999.1.1.1/push":                       true, // not an address literal: a DNS name, left to the sender
		"https://172.32.0.1/push":                      true, // just outside 172.16/12
		"https://[2001:4860:4860::8888]/push":          true,
		"https://example.com/?next=http://127.0.0.1/x": true,
	}
	for ep := range endpoints {
		one(`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth) VALUES ($1,$2,'k','a') RETURNING id`, uid, ep)
	}

	// --- issues ---
	wsA := one(`INSERT INTO workspaces (slug, name, key_prefix) VALUES ('cl-a','A','CLA') RETURNING id`)
	wsB := one(`INSERT INTO workspaces (slug, name, key_prefix) VALUES ('cl-b','B','CLB') RETURNING id`)
	stA := one(`INSERT INTO workflow_states (name, category, workspace_id) VALUES ('Backlog','backlog',$1) RETURNING id`, wsA)
	stB := one(`INSERT INTO workflow_states (name, category, workspace_id) VALUES ('Backlog','backlog',$1) RETURNING id`, wsB)
	projA := one(`INSERT INTO projects (name, workspace_id) VALUES ('pa',$1) RETURNING id`, wsA)
	projB := one(`INSERT INTO projects (name, workspace_id) VALUES ('pb',$1) RETURNING id`, wsB)

	issue := func(ws, st string, n int, key, title string, project, parent *string) string {
		return one(`INSERT INTO issues (workspace_id, number, key, title, description_md, state_id, project_id, parent_key, priority)
			VALUES ($1,$2,$3,$4,'keep me',$5,$6,$7,3) RETURNING id`, ws, n, key, title, st, project, parent)
	}
	str := func(s string) *string { return &s }
	issue(wsB, stB, 1, "CLB-1", "parent in B", nil, nil)
	issue(wsA, stA, 1, "CLA-1", "parent in A", nil, nil)
	foreignProject := issue(wsA, stA, 2, "CLA-2", "foreign project", &projB, nil)
	foreignParent := issue(wsA, stA, 3, "CLA-3", "foreign parent", nil, str("CLB-1"))
	goodProject := issue(wsA, stA, 4, "CLA-4", "good project", &projA, nil)
	goodParent := issue(wsA, stA, 5, "CLA-5", "good parent", nil, str("CLA-1"))
	danglingParent := issue(wsA, stA, 6, "CLA-6", "dangling parent", nil, str("CLA-999"))

	// Apply the migration SQL again over the fixtures (it ran on an empty
	// database during MigrateConfirmed).
	sqlBytes, err := migrationFS.ReadFile("migrations/" + cleanupMigration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("run %s: %v", cleanupMigration, err)
	}

	// Push subscriptions: bad deleted, good kept.
	for ep, kept := range endpoints {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE endpoint=$1`, ep).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if kept && n != 1 {
			t.Errorf("good endpoint %q was deleted", ep)
		}
		if !kept && n != 0 {
			t.Errorf("bad endpoint %q was kept", ep)
		}
	}

	// Issues: foreign references cleared, everything else kept.
	type row struct{ project, parent *string }
	get := func(id string) (r row, title, desc string, prio int) {
		t.Helper()
		if err := pool.QueryRow(ctx,
			`SELECT project_id::text, parent_key, title, description_md, priority FROM issues WHERE id=$1`, id).
			Scan(&r.project, &r.parent, &title, &desc, &prio); err != nil {
			t.Fatal(err)
		}
		return
	}
	r, title, desc, prio := get(foreignProject)
	if r.project != nil || title != "foreign project" || desc != "keep me" || prio != 3 {
		t.Errorf("foreign project issue: %+v %q %q %d", r, title, desc, prio)
	}
	r, title, desc, prio = get(foreignParent)
	if r.parent != nil || title != "foreign parent" || desc != "keep me" || prio != 3 {
		t.Errorf("foreign parent issue: %+v %q %q %d", r, title, desc, prio)
	}
	if r, _, _, _ = get(goodProject); r.project == nil || *r.project != projA {
		t.Errorf("good project reference changed: %+v", r)
	}
	if r, _, _, _ = get(goodParent); r.parent == nil || *r.parent != "CLA-1" {
		t.Errorf("good parent reference changed: %+v", r)
	}
	if r, _, _, _ = get(danglingParent); r.parent == nil || *r.parent != "CLA-999" {
		t.Errorf("dangling (unknown, not foreign) parent changed: %+v", r)
	}

	// A second run changes nothing.
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

// The migration deletes rows, so it must carry the destructive marker and ask
// for a backup on a database that already has data.
func TestSecurityCleanupIsGated(t *testing.T) {
	b, err := migrationFS.ReadFile("migrations/" + cleanupMigration)
	if err != nil {
		t.Fatal(err)
	}
	if !IsDestructive(string(b)) {
		t.Fatalf("%s deletes rows but is not marked destructive", cleanupMigration)
	}

	pool := scratchPool(t)
	ctx := context.Background()
	if err := MigrateConfirmed(ctx, pool, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version=$1`, cleanupMigration); err != nil {
		t.Fatal(err)
	}
	pending, err := PendingDestructive(ctx, pool)
	if err != nil || len(pending) != 1 || pending[0] != cleanupMigration {
		t.Fatalf("pending = %v, err %v", pending, err)
	}
	if err := MigrateConfirmed(ctx, pool, ""); err == nil {
		t.Fatal("migrated without a backup confirmation")
	}
	if err := MigrateConfirmed(ctx, pool, cleanupMigration); err != nil {
		t.Fatalf("confirmed migrate: %v", err)
	}
}

// What a migration reports with RAISE NOTICE reaches the log; other notices do not.
func TestLogMigrationNotice(t *testing.T) {
	// Must not panic on nil or unrelated notices.
	logMigrationNotice(nil, nil)
	logMigrationNotice(nil, &pgconn.Notice{Message: `relation "x" already exists, skipping`})
	logMigrationNotice(nil, &pgconn.Notice{Message: "migration 0039: 0 push subscription(s) deleted"})
}
