package store

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	if k.InProgress != 1 || k.Open != 1 || k.RunnersActive != 1 || k.MonthTokens != 1000 || k.MonthCostUSD != 0.25 {
		t.Errorf("kpis = %+v", k)
	}
	if len(d.Agents) != 1 || d.Agents[0].Runner != "claude" || d.Agents[0].IssueKey != a.Key {
		t.Errorf("agents = %+v", d.Agents)
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
	if _, err := s.FinishJob(ctx, wsA, job.ID, "other", "succeeded", nil, ""); !errors.Is(err, ErrConflict) {
		t.Errorf("a different host finished the job: %v", err)
	}
	done, err := s.FinishJob(ctx, wsA, job.ID, "mac", "succeeded", []byte(`{"ok":true}`), "")
	if err != nil || done.Status != "succeeded" || string(done.Result) != `{"ok": true}` {
		t.Errorf("finish: %v %+v %s", err, done.Status, done.Result)
	}

	h, err := s.HostHeartbeat(ctx, wsA, "mac", "1", []models.HarnessStatus{{Harness: "claude", Installed: true, Ready: true}})
	if err != nil || len(h.Harnesses) != 1 {
		t.Fatalf("heartbeat: %v %+v", err, h)
	}
	if hosts, _ := s.ListHosts(ctx, wsB); len(hosts) != 0 {
		t.Error("another workspace sees the host")
	}
}
