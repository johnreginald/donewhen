package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"raenil/internal/db"
	"raenil/internal/models"
)

// These tests need a throwaway Postgres. Set RAENIL_TEST_DATABASE_URL to run;
// otherwise they skip (so `go test ./...` stays green without a database).
func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("RAENIL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set RAENIL_TEST_DATABASE_URL to run store tests")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(pool, "K")
}

var wsCounter atomic.Int64

// newWorkspace makes an isolated workspace (states, labels and all) so tests
// never collide with each other or with real data in the test database.
func newWorkspace(t *testing.T, s *Store) string {
	t.Helper()
	ctx := context.Background()
	n := wsCounter.Add(1)
	// Prefixes must be unique across the whole database, not just this run.
	suffix := fmt.Sprintf("%d%d", time.Now().UnixNano()%100000, n)
	name := "Test " + suffix
	ws, err := s.CreateWorkspace(ctx, name, "test-"+suffix, "T"+suffix, "")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, ws.ID)
	})
	return ws.ID
}

func newUser(t *testing.T, s *Store) string {
	t.Helper()
	ctx := context.Background()
	email := fmt.Sprintf("t%d-%d@example.test", time.Now().UnixNano(), wsCounter.Add(1))
	u, err := s.CreateUser(ctx, email, "x")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })
	return u.ID
}

func TestIssueKeyIncrements(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	a, err := s.CreateIssue(ctx, ws, IssueInput{Title: "a", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateIssue(ctx, ws, IssueInput{Title: "b", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Key == b.Key {
		t.Fatalf("keys not unique: %s == %s", a.Key, b.Key)
	}
	if b.Number <= a.Number {
		t.Fatalf("number did not increment: %d then %d", a.Number, b.Number)
	}
}

// A new workspace numbers from 1 under its own prefix, independently of every
// other workspace — that is the whole point of per-workspace keys.
func TestKeysAreIndependentPerWorkspace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)

	a, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "a", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateIssue(ctx, wsB, IssueInput{Title: "b", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(a.Key, "-1") || !strings.HasSuffix(b.Key, "-1") {
		t.Fatalf("each workspace should start at 1, got %s and %s", a.Key, b.Key)
	}
	if strings.SplitN(a.Key, "-", 2)[0] == strings.SplitN(b.Key, "-", 2)[0] {
		t.Fatalf("workspaces shared a prefix: %s / %s", a.Key, b.Key)
	}
	if a.Number != b.Number {
		t.Fatalf("expected both to be number 1 (unique only per workspace), got %d and %d", a.Number, b.Number)
	}
}

// The isolation guarantee: nothing from one workspace is reachable from another.
func TestWorkspaceIsolation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)

	mine, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "private", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.GetIssue(ctx, wsB, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetIssue crossed workspaces: %v", err)
	}
	if _, err := s.GetIssueByKey(ctx, wsB, mine.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetIssueByKey crossed workspaces: %v", err)
	}
	list, err := s.ListIssues(ctx, IssueFilter{WorkspaceID: wsB})
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range list {
		if is.ID == mine.ID {
			t.Fatal("ListIssues returned another workspace's issue")
		}
	}
	if err := s.DeleteIssue(ctx, wsB, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteIssue crossed workspaces: %v", err)
	}
	title := "hijacked"
	if _, err := s.UpdateIssue(ctx, wsB, mine.ID, IssuePatch{Title: &title}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateIssue crossed workspaces: %v", err)
	}
	// And the original is untouched.
	still, err := s.GetIssue(ctx, wsA, mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.Title != "private" {
		t.Fatalf("issue was modified across the boundary: %q", still.Title)
	}
}

// A label name may repeat across workspaces, and attaching by name must never
// reach into another workspace's taxonomy.
func TestLabelsAreScopedPerWorkspace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)

	a, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "a", StateName: "Backlog", LabelNames: []string{"shared-name"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateIssue(ctx, wsB, IssueInput{Title: "b", StateName: "Backlog", LabelNames: []string{"shared-name"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Labels) != 1 || len(b.Labels) != 1 {
		t.Fatalf("expected one label each, got %d and %d", len(a.Labels), len(b.Labels))
	}
	if a.Labels[0].ID == b.Labels[0].ID {
		t.Fatal("the same label row was shared across workspaces")
	}
}

func TestMembershipGate(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	outsider := newUser(t, s)
	insider := newUser(t, s)

	if err := s.AddMember(ctx, ws, insider, "owner"); err != nil {
		t.Fatal(err)
	}
	if role, err := s.RoleIn(ctx, ws, insider); err != nil || role != "owner" {
		t.Fatalf("insider should be owner, got %q / %v", role, err)
	}
	if _, err := s.RoleIn(ctx, ws, outsider); !errors.Is(err, ErrNotMember) {
		t.Fatalf("outsider should not be a member, got %v", err)
	}
	// The last owner cannot be removed, or the workspace becomes unadministrable.
	if err := s.RemoveMember(ctx, ws, insider); err == nil {
		t.Fatal("removing the last owner should be refused")
	}
}

// A notification for one workspace must not reach a device whose owner is only
// a member of another.
func TestPushSubscriptionsFollowMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)
	alice, bob := newUser(t, s), newUser(t, s)

	if err := s.AddMember(ctx, wsA, alice, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(ctx, wsB, bob, "owner"); err != nil {
		t.Fatal(err)
	}
	aliceEP := fmt.Sprintf("https://push.test/alice-%d", time.Now().UnixNano())
	bobEP := fmt.Sprintf("https://push.test/bob-%d", time.Now().UnixNano())
	for _, sub := range []struct{ user, ep string }{{alice, aliceEP}, {bob, bobEP}} {
		if err := s.SavePushSubscription(ctx, sub.user, models.PushSubscription{
			Endpoint: sub.ep, P256dh: "k", Auth: "a",
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.DeletePushSubscriptionByEndpoint(ctx, sub.ep) })
	}

	subs, err := s.ListPushSubscriptions(ctx, wsA)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range subs {
		if sub.Endpoint == bobEP {
			t.Fatal("a non-member's device would have been notified")
		}
	}
	found := false
	for _, sub := range subs {
		if sub.Endpoint == aliceEP {
			found = true
		}
	}
	if !found {
		t.Fatal("the member's own device was not notified")
	}
}

// An agent reading across its memberships sees all of them at once, and each
// row says which workspace it came from.
func TestListIssuesSpansWorkspaces(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)

	a, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "in a", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateIssue(ctx, wsB, IssueInput{Title: "in b", StateName: "Ready"})
	if err != nil {
		t.Fatal(err)
	}

	both, err := s.ListIssues(ctx, IssueFilter{WorkspaceIDs: []string{wsA, wsB}})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, is := range both {
		seen[is.ID] = is.WorkspaceID
	}
	if seen[a.ID] != wsA || seen[b.ID] != wsB {
		t.Fatalf("expected both issues tagged with their own workspace, got %v", seen)
	}

	// A state name resolves across workspaces, where a state id could not.
	ready, err := s.ListIssues(ctx, IssueFilter{WorkspaceIDs: []string{wsA, wsB}, StateName: "Ready"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0].ID != b.ID {
		t.Fatalf("StateName filter across workspaces returned %d rows, want just the Ready one", len(ready))
	}

	// Narrowing to one workspace still excludes the other.
	onlyA, err := s.ListIssues(ctx, IssueFilter{WorkspaceID: wsA})
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range onlyA {
		if is.ID == b.ID {
			t.Fatal("single-workspace read leaked another workspace's issue")
		}
	}
}

// An agent asks by label name and wants what changed last: both must work
// across workspaces, where a label id or board position would not.
func TestListIssuesByLabelNameNewestFirst(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)

	old, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "old", StateName: "Backlog", LabelNames: []string{"Tagged"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "untagged", StateName: "Backlog"}); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.CreateIssue(ctx, wsB, IssueInput{Title: "fresh", StateName: "Backlog", LabelNames: []string{"tagged"}})
	if err != nil {
		t.Fatal(err)
	}
	title := "old, touched last"
	if _, err := s.UpdateIssue(ctx, wsA, old.ID, IssuePatch{Title: &title}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListIssues(ctx, IssueFilter{WorkspaceIDs: []string{wsA, wsB}, LabelName: "tagged", NewestFirst: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != old.ID || got[1].ID != fresh.ID {
		keys := make([]string, len(got))
		for i, is := range got {
			keys[i] = is.Key
		}
		t.Fatalf("got %v, want [%s %s]: both tagged issues, last updated first", keys, old.Key, fresh.Key)
	}
}

// Derivation: a bare key or id is enough to find the owning workspace, which is
// what lets a caller say "R-289" instead of naming a workspace.
func TestWorkspaceDerivation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "derive me", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{is.ID, is.Key, strings.ToLower(is.Key)} {
		got, err := s.WorkspaceOfIssueRef(ctx, ref)
		if err != nil {
			t.Fatalf("resolve %q: %v", ref, err)
		}
		if got != ws {
			t.Fatalf("resolve %q gave the wrong workspace", ref)
		}
	}
	if _, err := s.WorkspaceOfIssueRef(ctx, "NOPE-999"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown ref should be ErrNotFound, got %v", err)
	}

	ini, err := s.SaveInitiative(ctx, ws, models.Initiative{Name: "derive ini"})
	if err != nil {
		t.Fatal(err)
	}
	proj, err := s.SaveProject(ctx, ws, models.Project{Name: "derive epic", InitiativeID: &ini.ID})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.SaveDocument(ctx, ws, models.Document{Title: "derive doc", IssueID: &is.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ table, id string }{
		{"initiatives", ini.ID}, {"projects", proj.ID}, {"documents", doc.ID},
	} {
		got, err := s.WorkspaceOf(ctx, c.table, c.id)
		if err != nil || got != ws {
			t.Fatalf("WorkspaceOf(%s) = %q, %v", c.table, got, err)
		}
	}
	if _, err := s.WorkspaceOf(ctx, "users", is.ID); err == nil {
		t.Fatal("WorkspaceOf should refuse a table outside the tenant set")
	}
}

func TestExclusiveLabelGroupLastWins(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	// bug and feature are both in the exclusive 'type' group (seeded per workspace).
	is, err := s.CreateIssue(ctx, ws, IssueInput{
		Title:      "exclusive",
		StateName:  "Backlog",
		LabelNames: []string{"bug", "feature"},
	})
	if err != nil {
		t.Fatal(err)
	}
	typeLabels := 0
	for _, l := range is.Labels {
		if l.Name == "bug" || l.Name == "feature" {
			typeLabels++
		}
	}
	if typeLabels != 1 {
		t.Fatalf("expected exactly 1 label from exclusive 'type' group, got %d (%v)", typeLabels, is.Labels)
	}
	if is.Labels[0].Name != "feature" {
		t.Fatalf("expected last-wins 'feature', got %q", is.Labels[0].Name)
	}
}

func TestStateMovePersists(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "move me", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	name := "In Review"
	updated, err := s.UpdateIssue(ctx, ws, is.ID, IssuePatch{StateName: &name})
	if err != nil {
		t.Fatal(err)
	}
	if updated.StateID == is.StateID {
		t.Fatal("state did not change")
	}
	st, err := s.GetState(ctx, ws, updated.StateID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "In Review" {
		t.Fatalf("expected In Review, got %s", st.Name)
	}
}

// A workspace whose prefix already appears in legacy keys must step over them
// rather than fail — the retry loop in CreateIssue.
func TestKeyCollisionStepsOver(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	first, err := s.CreateIssue(ctx, ws, IssueInput{Title: "first", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	prefix := strings.SplitN(first.Key, "-", 2)[0]

	// Plant a squatter on the next key the workspace would hand out, as an
	// import or a pre-workspace issue could have done.
	var seq int64
	if err := s.pool.QueryRow(ctx, `SELECT issue_seq FROM workspaces WHERE id=$1`, ws).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	squatter := fmt.Sprintf("%s-%d", prefix, seq+1)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO issues (workspace_id, number, key, title, state_id)
		SELECT $1, 999999, $2, 'squatter', id FROM workflow_states
		WHERE workspace_id=$1 ORDER BY position LIMIT 1`, ws, squatter); err != nil {
		t.Fatal(err)
	}

	next, err := s.CreateIssue(ctx, ws, IssueInput{Title: "second", StateName: "Backlog"})
	if err != nil {
		t.Fatalf("create should have stepped over %s, got: %v", squatter, err)
	}
	if next.Key == squatter {
		t.Fatalf("reused the taken key %s", squatter)
	}
}

// Every runner the orchestrator can drive is a label a new workspace offers.
func TestNewWorkspaceOffersEveryRunner(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	labels, err := s.ListLabels(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, l := range labels {
		have[l.Name] = true
	}
	for _, runner := range []string{"opencode", "codex", "claude"} {
		if !have[runner] {
			t.Errorf("new workspace lacks the %q runner label", runner)
		}
	}
}

// A run is started, finished once, and stays inside its workspace.
func TestRunLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)
	is, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "work", StateName: "Ready"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.StartRun(ctx, wsB, RunStart{IssueID: is.ID, Runner: "claude"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("starting a run on another workspace's issue: err=%v, want ErrNotFound", err)
	}

	run, err := s.StartRun(ctx, wsA, RunStart{IssueID: is.ID, Runner: "claude", Model: "claude/haiku", Attempt: 2, Host: "mac"})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "running" || run.IssueKey != is.Key || run.Attempt != 2 || run.FinishedAt != nil {
		t.Fatalf("started run = %+v", run)
	}

	exit := 0
	done, err := s.FinishRun(ctx, wsA, run.ID, RunFinish{
		Status: "succeeded", Verdict: "pass", SessionID: "s1", ExitCode: &exit,
		Tokens:      models.RunTokens{Input: 10, CacheRead: 100, CacheCreation: 5, Output: 20},
		NotionalUSD: 0.02, Billing: "subscription", DeniedTools: []string{"Bash touch /tmp/x"}, LogTail: "the end",
	})
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "succeeded" || done.Tokens.Total != 135 || done.FinishedAt == nil ||
		len(done.DeniedTools) != 1 || done.NotionalUSD != 0.02 || done.CostUSD != 0 || done.LogTail != "the end" {
		t.Fatalf("finished run = %+v", done)
	}

	if _, err := s.FinishRun(ctx, wsA, run.ID, RunFinish{Status: "failed"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second finish: err=%v, want ErrConflict", err)
	}
	if _, err := s.FinishRun(ctx, wsA, run.ID, RunFinish{Status: "running"}); err == nil {
		t.Fatal("finishing with a non-final status was accepted")
	}

	list, err := s.ListRuns(ctx, wsA, RunFilter{IssueID: is.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].LogTail != "" {
		t.Fatalf("list = %+v, want one run without its log", list)
	}
	if other, _ := s.ListRuns(ctx, wsB, RunFilter{}); len(other) != 0 {
		t.Fatalf("another workspace sees %d runs", len(other))
	}
}

func TestDashboard(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	a, err := s.CreateIssue(ctx, ws, IssueInput{Title: "a", StateName: "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateIssue(ctx, ws, IssueInput{Title: "b", StateName: "Done"}); err != nil {
		t.Fatal(err)
	}
	run, err := s.StartRun(ctx, ws, RunStart{IssueID: a.ID, Runner: "claude", Model: "haiku"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishRun(ctx, ws, run.ID, RunFinish{Status: "succeeded", Verdict: "passed",
		Tokens: models.RunTokens{Total: 1000}, NotionalUSD: 0.5, CostUSD: 0.25, Billing: "api"}); err != nil {
		t.Fatal(err)
	}

	yangon, _ := time.LoadLocation("Asia/Yangon")
	d, err := s.Dashboard(ctx, ws, yangon)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Days) != 14 {
		t.Fatalf("days = %d, want 14", len(d.Days))
	}
	today := d.Days[13]
	if today.Date != time.Now().In(yangon).Format("2006-01-02") {
		t.Errorf("last day = %s, want today in Yangon", today.Date)
	}
	if today.RunsSucceeded != 1 || today.Passed != 1 || today.Finished != 1 {
		t.Errorf("today = %+v, want one succeeded, passed run", today)
	}
	k := d.KPIs
	if k.InProgress != 1 || k.Open != 1 || k.MonthTokens != 1000 || k.MonthCostUSD != 0.25 {
		t.Errorf("kpis = %+v", k)
	}
	// Runs with no agent do not make one up: the strip shows agents.
	if len(d.Agents) != 0 {
		t.Errorf("agents = %+v, want none: no agent exists", d.Agents)
	}
	if len(d.Recent) != 2 {
		t.Errorf("recent tasks = %d, want 2", len(d.Recent))
	}
}

func TestAgentsAndJobs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)
	str := func(v string) *string { return &v }

	a1, err := s.CreateAgent(ctx, wsA, AgentInput{Name: str("Chief Engineer"), Harness: str("claude"), Model: str("sonnet")})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.CreateAgent(ctx, wsA, AgentInput{Name: str("Chief Engineer"), Harness: str("codex")})
	if err != nil {
		t.Fatal(err)
	}
	if a1.Slug != "chief-engineer" || a2.Slug != "chief-engineer-2" {
		t.Errorf("slugs = %q, %q", a1.Slug, a2.Slug)
	}
	if _, err := s.CreateAgent(ctx, wsA, AgentInput{Name: str("x"), Harness: str("gpt")}); err == nil {
		t.Error("an unknown harness was accepted")
	}
	if _, err := s.UpdateAgent(ctx, wsA, a1.ID, AgentInput{AllowedTools: []string{"Bash"}, SetAllowed: true}); err == nil {
		t.Error("bare Bash was accepted as an allowed tool")
	}
	if got, err := s.GetAgent(ctx, wsA, "chief-engineer"); err != nil || got.ID != a1.ID {
		t.Errorf("get by slug: %v %v", got.ID, err)
	}
	if _, err := s.GetAgent(ctx, wsB, a1.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another workspace read the agent: %v", err)
	}

	// A ticket can be for an agent in its own workspace only.
	is, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "t", StateName: "Ready", AgentID: &a1.ID})
	if err != nil || is.AgentID == nil || *is.AgentID != a1.ID {
		t.Fatalf("create with agent: %v %+v", err, is.AgentID)
	}
	other, _ := s.CreateIssue(ctx, wsB, IssueInput{Title: "o", StateName: "Ready"})
	if _, err := s.UpdateIssue(ctx, wsB, other.ID, IssuePatch{AgentID: &a1.ID, SetAgent: true}); !errors.Is(err, ErrNotFound) {
		t.Errorf("assigned another workspace's agent: %v", err)
	}

	// Jobs: a host only gets work for a harness it has, once, and only it can finish it.
	job, err := s.EnqueueJob(ctx, wsA, JobInput{Kind: "run_ticket", AgentID: a1.ID, IssueID: is.ID})
	if err != nil || job.Status != "queued" || job.IssueKey != is.Key {
		t.Fatalf("enqueue: %v %+v", err, job)
	}
	if _, ok, _ := s.ClaimJob(ctx, wsA, "mac", []string{"codex"}); ok {
		t.Error("a codex-only host claimed a claude job")
	}
	got, ok, err := s.ClaimJob(ctx, wsA, "mac", []string{"claude"})
	if err != nil || !ok || got.ID != job.ID || got.Status != "claimed" {
		t.Fatalf("claim: %v %v %+v", ok, err, got)
	}
	if _, ok, _ := s.ClaimJob(ctx, wsA, "other", []string{"claude"}); ok {
		t.Error("a claimed job was claimed again")
	}
	if _, err := s.EnqueueJob(ctx, wsA, JobInput{Kind: "run_ticket", AgentID: a1.ID, IssueID: is.ID}); !errors.Is(err, ErrConflict) {
		t.Errorf("a ticket was queued twice while running: %v", err)
	}
	if _, err := s.FinishJob(ctx, wsA, job.ID, "other", "succeeded", nil, ""); !errors.Is(err, ErrConflict) {
		t.Errorf("a different host finished the job: %v", err)
	}
	done, err := s.FinishJob(ctx, wsA, job.ID, "mac", "succeeded", []byte(`{"ok":true}`), "")
	if err != nil || done.Status != "succeeded" || string(done.Result) != `{"ok": true}` {
		t.Errorf("finish: %v %+v %s", err, done.Status, done.Result)
	}

	if _, err := s.EnqueueJob(ctx, wsA, JobInput{Kind: "run_ticket", AgentID: a1.ID, IssueID: is.ID}); err != nil {
		t.Errorf("a finished ticket could not be run again: %v", err)
	}

	h, err := s.HostHeartbeat(ctx, wsA, "mac", "1", []models.HarnessStatus{{Harness: "claude", Installed: true, Ready: true}})
	if err != nil || len(h.Harnesses) != 1 {
		t.Fatalf("heartbeat: %v %+v", err, h)
	}
	if hosts, _ := s.ListHosts(ctx, wsB); len(hosts) != 0 {
		t.Error("another workspace sees the host")
	}
}

func TestConversationPieces(t *testing.T) {
	// Validation needs no database.
	if _, err := NormaliseQuestions(nil); err == nil {
		t.Error("no questions was accepted")
	}
	qs, err := NormaliseQuestions([]models.Question{{Text: " Window? ", Options: []string{"minute", " ", "hour"}}, {Text: "Anything else?"}})
	if err != nil || qs[0].ID != "q1" || len(qs[0].Options) != 2 || !qs[1].AllowOther {
		t.Fatalf("normalise: %v %+v", err, qs)
	}
	if err := CheckAnswers(qs, []models.Answer{{QuestionID: "q1", Choices: []string{"day"}}, {QuestionID: "q2", Other: "no"}}); err == nil {
		t.Error("an answer outside the options was accepted")
	}
	if err := CheckAnswers(qs, []models.Answer{{QuestionID: "q1", Choices: []string{"minute"}}}); err == nil {
		t.Error("an unanswered question was accepted")
	}
	if err := CheckAnswers(qs, []models.Answer{{QuestionID: "q1", Choices: []string{"minute"}}, {QuestionID: "q2", Other: "no"}}); err != nil {
		t.Errorf("good answers refused: %v", err)
	}
	ungated := Proposal{Tickets: []ProposedTicket{{Title: "t", Criteria: []ProposedCriterion{{Text: "looks right", Kind: "manual"}}}}}
	if _, err := NormaliseProposal(ungated); err == nil {
		t.Error("a ticket with nothing a program can check was accepted")
	}
	gated := Proposal{Tickets: []ProposedTicket{{Title: "t", Criteria: []ProposedCriterion{
		{Text: "tests pass", Kind: "deterministic", Check: json.RawMessage(`{"cmd":"go test ./..."}`)}}}}}
	if _, err := NormaliseProposal(gated); err != nil {
		t.Errorf("a gated proposal was refused: %v", err)
	}

	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	name, harness := "Eng", "claude"
	a, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &name, Harness: &harness})
	is, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "t", StateName: "Aligning"})

	it, err := s.CreateInteraction(ctx, ws, is.ID, a.ID, "questions", map[string]any{"questions": qs})
	if err != nil || it.Status != "open" || it.AgentID == nil {
		t.Fatalf("create interaction: %v %+v", err, it)
	}
	if _, err := s.ResolveInteraction(ctx, ws, it.ID, "answered", map[string]any{"answers": []models.Answer{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveInteraction(ctx, ws, it.ID, "answered", nil); !errors.Is(err, ErrConflict) {
		t.Errorf("an interaction was answered twice: %v", err)
	}

	if _, err := s.SaveAgentSession(ctx, ws, a.ID, is.ID, "s1", "/repo", "fp"); err != nil {
		t.Fatal(err)
	}
	ss, err := s.SaveAgentSession(ctx, ws, a.ID, is.ID, "s1", "/repo", "fp")
	if err != nil || ss.SessionID != "s1" || ss.Turns != 2 {
		t.Errorf("session after two turns: %v %+v", err, ss)
	}
	// A new session starts the count again.
	ss, err = s.SaveAgentSession(ctx, ws, a.ID, is.ID, "s2", "/repo", "fp2")
	if err != nil || ss.SessionID != "s2" || ss.Turns != 1 || ss.Fingerprint != "fp2" {
		t.Errorf("a new session kept the old count: %v %+v", err, ss)
	}

	// Messages while a turn waits join it.
	j1, _ := s.EnqueueJob(ctx, ws, JobInput{Kind: "chat", AgentID: a.ID, IssueID: is.ID})
	j2, _ := s.EnqueueJob(ctx, ws, JobInput{Kind: "chat", AgentID: a.ID, IssueID: is.ID})
	if j1.ID == "" || j1.ID != j2.ID {
		t.Errorf("a second message queued another turn: %s vs %s", j1.ID, j2.ID)
	}
}

func TestReapingWorkOfGoneHosts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	name, harness := "Eng", "claude"
	a, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &name, Harness: &harness})
	is1, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "one", StateName: "Ready"})
	is2, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "two", StateName: "Ready"})

	// A restarted host releases what it held.
	if _, err := s.HostHeartbeat(ctx, ws, "mac", "", nil); err != nil {
		t.Fatal(err)
	}
	j1, _ := s.EnqueueJob(ctx, ws, JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: is1.ID})
	if _, ok, _ := s.ClaimJob(ctx, ws, "mac", []string{"claude"}); !ok {
		t.Fatal("claim failed")
	}
	run, _ := s.StartRun(ctx, ws, RunStart{IssueID: is1.ID, Runner: "claude", Host: "mac"})
	jobs, runs, err := s.ReleaseHost(ctx, ws, "mac", "restarted")
	if err != nil || len(jobs) != 1 || jobs[0].ID != j1.ID || jobs[0].Status != "failed" ||
		len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status != "aborted" {
		t.Fatalf("release: %v jobs=%+v runs=%+v", err, jobs, runs)
	}
	if _, err := s.EnqueueJob(ctx, ws, JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: is1.ID}); err != nil {
		t.Errorf("the ticket stayed locked after its host restarted: %v", err)
	}

	// A host that went silent has its claim reaped.
	if _, err := s.HostHeartbeat(ctx, ws, "old", "", nil); err != nil {
		t.Fatal(err)
	}
	j2, _ := s.EnqueueJob(ctx, ws, JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: is2.ID})
	for {
		j, ok, _ := s.ClaimJob(ctx, ws, "old", []string{"claude"})
		if !ok || j.ID == j2.ID {
			break
		}
	}
	if _, err := s.pool.Exec(ctx, `UPDATE runner_hosts SET last_seen_at = now() - interval '1 hour' WHERE workspace_id = $1 AND name = 'old'`, ws); err != nil {
		t.Fatal(err)
	}
	byWS, err := s.ReapStale(ctx, 5*time.Minute, 3*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Everything "old" had claimed is reaped, with a reason.
	got := byWS[ws]
	found := false
	for _, j := range got.Jobs {
		found = found || j.ID == j2.ID
		if j.Host != "old" || j.Status != "failed" || j.Error == "" {
			t.Errorf("reaped job %+v", j)
		}
	}
	if !found {
		t.Errorf("the silent host's job was not reaped: %+v", got.Jobs)
	}
}

func TestHeartbeats(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	name, harness := "Eng", "claude"
	a, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &name, Harness: &harness})
	is, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "Bump deps", StateName: "Ready", AgentID: &a.ID})

	// Heartbeat: a human message since the agent last spoke is waiting for it.
	hb := 5
	s.UpdateAgent(ctx, ws, a.ID, AgentInput{Heartbeat: &hb})
	if _, err := s.UpdateAgent(ctx, ws, a.ID, AgentInput{Heartbeat: str2int(2)}); err == nil {
		t.Error("a two-minute heartbeat was accepted")
	}
	s.CreateComment(ctx, ws, is.ID, "reply to me", "human", "")
	beats, _ := s.ClaimDueHeartbeats(ctx, time.Now())
	found := false
	for _, b := range beats {
		found = found || b.AgentID == a.ID
	}
	if !found {
		t.Fatal("a due heartbeat was not claimed")
	}
	if b2, _ := s.ClaimDueHeartbeats(ctx, time.Now()); len(b2) > 0 {
		for _, b := range b2 {
			if b.AgentID == a.ID {
				t.Error("a heartbeat fired twice in one interval")
			}
		}
	}
	waiting, _ := s.WaitingForAgent(ctx, ws, a.ID)
	if len(waiting) != 1 || waiting[0] != is.ID {
		t.Errorf("waiting = %v, want the ticket with the new message", waiting)
	}
	s.CreateComment(ctx, ws, is.ID, "done", "ai", a.ID)
	if waiting, _ := s.WaitingForAgent(ctx, ws, a.ID); len(waiting) != 0 {
		t.Errorf("still waiting after the agent replied: %v", waiting)
	}
}

func str2int(n int) *int { return &n }

func TestCostsAndBudgets(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	name, harness := "Eng", "claude"
	a, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &name, Harness: &harness})
	is, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "t", StateName: "Ready", AgentID: &a.ID})
	for _, f := range []RunFinish{
		{Status: "succeeded", Billing: "subscription", NotionalUSD: 0.5, Tokens: models.RunTokens{Input: 100, CacheRead: 900}},
		{Status: "failed", Billing: "api", CostUSD: 0.25, Tokens: models.RunTokens{Input: 50, Output: 50}},
	} {
		run, _ := s.StartRun(ctx, ws, RunStart{IssueID: is.ID, AgentID: a.ID, Runner: "claude"})
		if _, err := s.FinishRun(ctx, ws, run.ID, f); err != nil {
			t.Fatal(err)
		}
	}
	c, err := s.Costs(ctx, ws, MonthStart(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Total.Runs != 2 || c.Total.Tokens != 1100 || c.Total.CacheRead != 900 || c.Total.CostUSD != 0.25 ||
		c.Total.NotionalUSD != 0.5 || c.Total.SubscriptionRuns != 1 || c.Total.APIRuns != 1 {
		t.Errorf("total = %+v", c.Total)
	}
	if len(c.ByAgent) != 1 || c.ByAgent[0].Name != "Eng" || len(c.ByTicket) != 1 || c.ByTicket[0].Key != is.Key {
		t.Errorf("by agent %+v, by ticket %+v", c.ByAgent, c.ByTicket)
	}
	u, _ := s.AgentMonthUsage(ctx, ws, a.ID, time.Now())
	if share := BudgetShare(2000, 0, u); share < 0.54 || share > 0.56 {
		t.Errorf("token share = %v, want 0.55", share)
	}
	if share := BudgetShare(0, 0.2, u); share < 1 {
		t.Errorf("dollar share = %v, want over 1", share)
	}

	// A paused agent's queued work is not handed to a host.
	j, _ := s.EnqueueJob(ctx, ws, JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: is.ID})
	paused := "paused"
	s.UpdateAgent(ctx, ws, a.ID, AgentInput{Status: &paused})
	if got, ok, _ := s.ClaimJob(ctx, ws, "mac", []string{"claude"}); ok && got.ID == j.ID {
		t.Error("a paused agent's job was handed out")
	}
}

func TestBudgetGuard(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	// An agent at its cap takes no new work.
	name, harness := "Eng", "claude"
	a, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &name, Harness: &harness})
	is, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "t", StateName: "Ready"})
	run, _ := s.StartRun(ctx, ws, RunStart{IssueID: is.ID, AgentID: a.ID, Runner: "claude"})
	s.FinishRun(ctx, ws, run.ID, RunFinish{Status: "succeeded", Tokens: models.RunTokens{Input: 5000}})
	cap := int64(1000)
	s.UpdateAgent(ctx, ws, a.ID, AgentInput{BudgetTokens: &cap})
	if _, err := s.EnqueueJob(ctx, ws, JobInput{Kind: "chat", AgentID: a.ID, IssueID: is.ID}); !errors.Is(err, ErrConflict) {
		t.Errorf("an over-budget agent was given work: %v", err)
	}
	if _, err := s.EnqueueJob(ctx, ws, JobInput{Kind: "test_env", AgentID: a.ID}); err != nil {
		t.Errorf("an environment test was refused for budget: %v", err)
	}
}

// A task with no agent goes to the workspace's default for its priority; one
// chosen on the task wins; clearing the default leaves it with none.
func TestPriorityAgents(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	name, other, harness := "Opus", "Cheap", "claude"
	opus, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &name, Harness: &harness})
	cheap, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &other, Harness: &harness})

	if err := s.SetPriorityAgent(ctx, ws, 1, opus.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPriorityAgent(ctx, ws, 4, cheap.Slug); err != nil {
		t.Fatalf("by slug: %v", err)
	}
	if err := s.SetPriorityAgent(ctx, ws, 7, opus.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("priority 7 accepted: %v", err)
	}
	urgent, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "u", StateName: "Ready", Priority: 1})
	low, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "l", StateName: "Ready", Priority: 4})
	none, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "n", StateName: "Ready"})
	chosen, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "c", StateName: "Ready", Priority: 1, AgentID: &cheap.ID})

	for _, c := range []struct {
		is   models.Issue
		want string
	}{{urgent, opus.ID}, {low, cheap.ID}, {none, ""}, {chosen, cheap.ID}} {
		if got, err := s.IssueAgentRef(ctx, ws, c.is); err != nil || got != c.want {
			t.Errorf("%s: agent = %q (%v), want %q", c.is.Title, got, err, c.want)
		}
	}
	if list, _ := s.ListPriorityAgents(ctx, ws); len(list) != 2 {
		t.Errorf("list = %+v", list)
	}
	s.SetPriorityAgent(ctx, ws, 1, "")
	if got, _ := s.IssueAgentRef(ctx, ws, urgent); got != "" {
		t.Errorf("cleared default still applies: %q", got)
	}
	// Another workspace's agent cannot be a default here.
	ws2 := newWorkspace(t, s)
	if err := s.SetPriorityAgent(ctx, ws2, 2, opus.ID); err == nil {
		t.Error("an agent from another workspace was accepted")
	}
}

// A ticket's blockers are links: open ones hold it back, a cycle is refused,
// and a running epic offers only the Ready tickets nothing open blocks.
func TestBlockersAndEpicRun(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	epic, err := s.SaveProject(ctx, ws, models.Project{Name: "E1", Status: "planned"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(title, state string) models.Issue {
		is, err := s.CreateIssue(ctx, ws, IssueInput{Title: title, StateName: state, ProjectID: &epic.ID})
		if err != nil {
			t.Fatal(err)
		}
		return is
	}
	base := mk("base", "Ready")
	next := mk("next", "Ready")
	free := mk("free", "Ready")
	mk("aligning", "Aligning")

	if _, err := s.SetBlockers(ctx, ws, next.ID, []string{base.Key}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBlockers(ctx, ws, base.ID, []string{next.Key}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a cycle was accepted: %v", err)
	}
	if _, err := s.SetBlockers(ctx, ws, base.ID, []string{base.Key}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a ticket blocked itself: %v", err)
	}
	if open, _ := s.OpenBlockers(ctx, ws, next.ID); len(open) != 1 || open[0].Key != base.Key {
		t.Errorf("open blockers = %+v", open)
	}
	if blocking, _ := s.ListBlocking(ctx, ws, base.ID); len(blocking) != 1 || blocking[0].Key != next.Key {
		t.Errorf("blocking = %+v", blocking)
	}

	ready, blocked, err := s.ReadyToRun(ctx, ws, epic.ID)
	if err != nil {
		t.Fatal(err)
	}
	keys := func(l []models.Issue) (out []string) {
		for _, i := range l {
			out = append(out, i.Key)
		}
		return
	}
	if got := keys(ready); len(got) != 2 || !slices.Contains(got, base.Key) || !slices.Contains(got, free.Key) {
		t.Errorf("ready = %v, want %s and %s", got, base.Key, free.Key)
	}
	if len(blocked) != 1 || blocked[0] != next.Key {
		t.Errorf("blocked = %v, want %s", blocked, next.Key)
	}

	// In Review is far enough: the dependent builds on the blocker's branch.
	review := "In Review"
	s.UpdateIssue(ctx, ws, base.ID, IssuePatch{StateName: &review})
	if open, _ := s.OpenBlockers(ctx, ws, next.ID); len(open) != 0 {
		t.Errorf("an In Review blocker still blocks: %+v", open)
	}
	if _, blocked, _ := s.ReadyToRun(ctx, ws, epic.ID); len(blocked) != 0 {
		t.Errorf("still blocked with the blocker In Review: %v", blocked)
	}

	// The blocker is Done: the next ticket is free, and a running epic
	// names itself as the one to restart.
	s.SetEpicAutorun(ctx, ws, epic.ID, true)
	done := "Done"
	s.UpdateIssue(ctx, ws, base.ID, IssuePatch{StateName: &done})
	if open, _ := s.OpenBlockers(ctx, ws, next.ID); len(open) != 0 {
		t.Errorf("a Done blocker still blocks: %+v", open)
	}
	if epics, _ := s.AutorunDependents(ctx, ws, base.ID); len(epics) != 1 || epics[0] != epic.ID {
		t.Errorf("autorun epics = %v", epics)
	}
	s.SetEpicAutorun(ctx, ws, epic.ID, false)
	if epics, _ := s.AutorunDependents(ctx, ws, base.ID); len(epics) != 0 {
		t.Errorf("a stopped epic still restarts: %v", epics)
	}
}

// The workspace's shared rules are kept trimmed and unique, and refuse a rule
// that would allow any shell command.
func TestWorkspaceAllowedTools(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	if got, _ := s.WorkspaceAllowedTools(ctx, ws); len(got) != 0 {
		t.Errorf("a new workspace has rules: %v", got)
	}
	got, err := s.SetWorkspaceAllowedTools(ctx, ws, []string{" Bash(curl *) ", "Bash(curl *)", "", "Bash(cp *)"})
	if err != nil || len(got) != 2 || got[0] != "Bash(curl *)" {
		t.Errorf("set = %v, %v", got, err)
	}
	if _, err := s.SetWorkspaceAllowedTools(ctx, ws, []string{"Bash(*)"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("Bash(*) accepted: %v", err)
	}
	if got, _ := s.WorkspaceAllowedTools(ctx, ws); len(got) != 2 {
		t.Errorf("a refused update changed the rules: %v", got)
	}
}

// Stopping a queued job cancels it; stopping one a host works on marks it for
// the host to end; a finished job is left alone.
func TestStopJob(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	name, harness := "Eng", "claude"
	a, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &name, Harness: &harness})
	is, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "t", StateName: "Ready", AgentID: &a.ID})

	q, _ := s.EnqueueJob(ctx, ws, JobInput{Kind: "chat", AgentID: a.ID, IssueID: is.ID})
	if j, err := s.StopJob(ctx, ws, q.ID); err != nil || j.Status != "canceled" {
		t.Errorf("queued job after stop = %s, %v", j.Status, err)
	}

	w, _ := s.EnqueueJob(ctx, ws, JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: is.ID})
	if _, ok, _ := s.ClaimJob(ctx, ws, "mac", []string{"claude"}); !ok {
		t.Fatal("not claimed")
	}
	if ids, _ := s.ActiveJobsFor(ctx, ws, is.ID); len(ids) != 1 || ids[0] != w.ID {
		t.Errorf("active = %v", ids)
	}
	j, _ := s.StopJob(ctx, ws, w.ID)
	if j.Status != "claimed" || !j.StopRequested {
		t.Errorf("working job after stop = %s stop=%v", j.Status, j.StopRequested)
	}
	if f, err := s.FinishJob(ctx, ws, w.ID, "mac", "canceled", nil, "stopped by you"); err != nil || f.Status != "canceled" {
		t.Errorf("host could not report the stop: %v %v", f.Status, err)
	}
	if j, _ := s.StopJob(ctx, ws, w.ID); j.Status != "canceled" {
		t.Errorf("a finished job changed: %s", j.Status)
	}
}

// Hosts run jobs in parallel, but never two for one ticket at a time.
func TestClaimOneJobPerTicket(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	name, harness := "Eng", "claude"
	a, _ := s.CreateAgent(ctx, ws, AgentInput{Name: &name, Harness: &harness})
	one, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "one", StateName: "Ready", AgentID: &a.ID})
	two, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "two", StateName: "Ready", AgentID: &a.ID})
	s.EnqueueJob(ctx, ws, JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: one.ID})
	s.EnqueueJob(ctx, ws, JobInput{Kind: "chat", AgentID: a.ID, IssueID: one.ID})
	s.EnqueueJob(ctx, ws, JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: two.ID})

	first, ok, _ := s.ClaimJob(ctx, ws, "mac", []string{"claude"})
	second, ok2, _ := s.ClaimJob(ctx, ws, "mac", []string{"claude"})
	if !ok || !ok2 || *first.IssueID == *second.IssueID {
		t.Fatalf("claimed %v and %v: want one job from each ticket", first.IssueKey, second.IssueKey)
	}
	if _, ok, _ := s.ClaimJob(ctx, ws, "mac", []string{"claude"}); ok {
		t.Error("a second job for a ticket already being worked was handed out")
	}
	s.FinishJob(ctx, ws, first.ID, "mac", "succeeded", nil, "")
	if j, ok, _ := s.ClaimJob(ctx, ws, "mac", []string{"claude"}); !ok || *j.IssueID != one.ID {
		t.Error("the ticket's next job was not handed out once the first finished")
	}
}
