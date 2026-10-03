package db

import (
	"context"
	"testing"
)

// DW-91: the migration deletes the labels of every `runner` group, then the group.
// Tickets, documents and other labels stay, and so does a label that only has a
// runner-like name in another group.
func TestRemoveRunnerLabelsMigration(t *testing.T) {
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
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}

	a := one(`INSERT INTO workspaces (slug, name, key_prefix) VALUES ('rl-a','A','RLA') RETURNING id`)
	b := one(`INSERT INTO workspaces (slug, name, key_prefix) VALUES ('rl-b','B','RLB') RETURNING id`)
	runnerGroup := one(`INSERT INTO label_groups (workspace_id, name, exclusive) VALUES ($1,'runner',true) RETURNING id`, a)
	typeA := one(`INSERT INTO label_groups (workspace_id, name, exclusive) VALUES ($1,'type',true) RETURNING id`, a)
	typeB := one(`INSERT INTO label_groups (workspace_id, name, exclusive) VALUES ($1,'type',true) RETURNING id`, b)
	codex := one(`INSERT INTO labels (workspace_id, group_id, name, color) VALUES ($1,$2,'codex','#888') RETURNING id`, a, runnerGroup)
	exec(`INSERT INTO labels (workspace_id, group_id, name, color) VALUES ($1,$2,'opencode','#888')`, a, runnerGroup)
	bug := one(`INSERT INTO labels (workspace_id, group_id, name, color) VALUES ($1,$2,'bug','#888') RETURNING id`, a, typeA)
	exec(`INSERT INTO labels (workspace_id, group_id, name, color) VALUES ($1,$2,'codex','#888')`, b, typeB)

	state := one(`INSERT INTO workflow_states (workspace_id, name, category, position, color) VALUES ($1,'Backlog','backlog',1,'#999') RETURNING id`, a)
	issue := one(`INSERT INTO issues (workspace_id, number, key, title, state_id) VALUES ($1,1,'RLA-1','t',$2) RETURNING id`, a, state)
	doc := one(`INSERT INTO documents (workspace_id, title) VALUES ($1,'d') RETURNING id`, a)
	exec(`INSERT INTO issue_labels (issue_id, label_id) VALUES ($1,$2),($1,$3)`, issue, codex, bug)
	exec(`INSERT INTO document_labels (document_id, label_id) VALUES ($1,$2)`, doc, codex)

	sqlBytes, err := migrationFS.ReadFile("migrations/0050_remove_runner_labels.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !IsDestructive(string(sqlBytes)) {
		t.Fatal("0050 deletes data and must carry the destructive marker")
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatal(err)
	}

	if n := count(`SELECT count(*) FROM label_groups WHERE name='runner'`); n != 0 {
		t.Fatalf("runner groups left: %d", n)
	}
	if n := count(`SELECT count(*) FROM labels WHERE group_id=$1`, runnerGroup); n != 0 {
		t.Fatalf("runner labels left: %d", n)
	}
	if n := count(`SELECT count(*) FROM labels WHERE workspace_id=$1 AND name IN ('codex','opencode')`, a); n != 0 {
		t.Fatalf("labels with no group left behind: %d", n)
	}
	// What must stay.
	if n := count(`SELECT count(*) FROM labels WHERE id=$1`, bug); n != 1 {
		t.Fatal("the type label was deleted")
	}
	if n := count(`SELECT count(*) FROM labels WHERE workspace_id=$1 AND name='codex' AND group_id=$2`, b, typeB); n != 1 {
		t.Fatal("a codex label in the type group was deleted")
	}
	if count(`SELECT count(*) FROM issues WHERE id=$1`, issue) != 1 || count(`SELECT count(*) FROM documents WHERE id=$1`, doc) != 1 {
		t.Fatal("the issue or the document was deleted")
	}
	// The links to the deleted labels are gone; the link to the type label stays.
	if n := count(`SELECT count(*) FROM issue_labels WHERE issue_id=$1`, issue); n != 1 {
		t.Fatalf("issue_labels = %d, want 1 (only bug)", n)
	}
	if n := count(`SELECT count(*) FROM document_labels WHERE document_id=$1`, doc); n != 0 {
		t.Fatalf("document_labels = %d, want 0", n)
	}
}
