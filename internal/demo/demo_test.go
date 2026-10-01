package demo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/db"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/service"
	"github.com/johnreginald/donewhen/internal/store"
)

func TestSeedCountsAndIdempotent(t *testing.T) {
	dsn := config.Getenv("DONEWHEN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DONEWHEN_TEST_DATABASE_URL to run demo tests")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	st := store.New(pool, "K")
	svc := service.New(st, events.NewBus())

	// A unique key prefix, so the test never touches a real DEMO workspace.
	old := prefix
	prefix = fmt.Sprintf("T%c%c", 'A'+byte(time.Now().UnixNano()%26), 'A'+byte(time.Now().UnixNano()/26%26))
	t.Cleanup(func() { prefix = old })
	email := fmt.Sprintf("demo-%d@example.com", time.Now().UnixNano())
	u, err := st.CreateUser(ctx, email, "x")
	if err != nil {
		t.Fatal(err)
	}
	slug := fmt.Sprintf("demotest%d", time.Now().UnixNano()%100000)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE slug=$1`, slug)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID)
	})

	res, err := Seed(ctx, svc, u.ID, slug)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := st.ResolveWorkspace(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	if res.Epics != 3 {
		t.Errorf("epics = %d, want 3", res.Epics)
	}
	if res.Issues < 22 || res.Issues > 26 {
		t.Errorf("issues = %d, want about 24", res.Issues)
	}
	if res.Documents != 3 {
		t.Errorf("documents = %d, want 3", res.Documents)
	}

	// Every state is used.
	states, _ := st.ListStates(ctx, ws.ID)
	all, err := st.ListIssues(ctx, store.IssueFilter{WorkspaceID: ws.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != res.Issues {
		t.Errorf("listed %d issues, seeded %d", len(all), res.Issues)
	}
	perState := map[string]int{}
	for _, is := range all {
		perState[is.StateID]++
	}
	for _, s := range states {
		if perState[s.ID] == 0 {
			t.Errorf("no issue in state %s", s.Name)
		}
	}

	// The Inbox lists the 3 In Review issues, each with a complete checklist.
	inbox, err := st.InboxNeedsReview(ctx, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 3 {
		t.Fatalf("inbox = %d, want 3", len(inbox))
	}
	for _, it := range inbox {
		if it.Issue.CriteriaTotal == 0 || it.Issue.CriteriaDone != it.Issue.CriteriaTotal {
			t.Errorf("%s checklist %d/%d, want complete", it.Issue.Key, it.Issue.CriteriaDone, it.Issue.CriteriaTotal)
		}
	}
	links, err := st.ListBlockLinks(ctx, ws.ID)
	if err != nil || len(links) != 2 {
		t.Errorf("block links = %d (%v), want 2", len(links), err)
	}

	count := func() int {
		var n int
		_ = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM issues WHERE workspace_id=$1)
			+ (SELECT count(*) FROM activity WHERE workspace_id=$1)
			+ (SELECT count(*) FROM documents WHERE workspace_id=$1)
			+ (SELECT count(*) FROM projects WHERE workspace_id=$1)`, ws.ID).Scan(&n)
		return n
	}
	before := count()
	if _, err := Seed(ctx, svc, u.ID, slug); !errors.Is(err, ErrExists) {
		t.Fatalf("second run err = %v, want ErrExists", err)
	}
	if after := count(); after != before {
		t.Errorf("second run changed rows: %d -> %d", before, after)
	}
}
