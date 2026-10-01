package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/johnreginald/donewhen/internal/testdb"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnreginald/donewhen/internal/db"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

var wsSeq atomic.Int64

type gateEnv struct {
	t   *testing.T
	svc *Service
	ws  string
	ctx context.Context
}

func newGateEnv(t *testing.T) *gateEnv {
	t.Helper()
	dsn := testdb.DSN(t)
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
	sfx := fmt.Sprintf("%d%d", time.Now().UnixNano()%100000, wsSeq.Add(1))
	w, err := st.CreateWorkspace(ctx, "Gate "+sfx, "gate-"+sfx, "G"+sfx, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, w.ID) })
	return &gateEnv{t: t, svc: New(st, events.NewBus()), ws: w.ID, ctx: ctx}
}

func (e *gateEnv) issue(state string) models.Issue {
	e.t.Helper()
	is, err := e.svc.CreateIssue(e.ctx, e.ws, store.IssueInput{Title: "t", StateName: state}, "human")
	if err != nil {
		e.t.Fatal(err)
	}
	return is
}

func (e *gateEnv) crit(issueID, body, kind string, done bool) models.Criterion {
	e.t.Helper()
	var spec []byte
	if kind == models.CriterionJudgment {
		spec = []byte(`{"prompt":"p"}`)
	}
	c, err := e.svc.Store.AddCriterion(e.ctx, e.ws, issueID, body, kind, spec)
	if err != nil {
		e.t.Fatal(err)
	}
	if done {
		d := true
		if c, err = e.svc.Store.UpdateCriterion(e.ctx, e.ws, c.ID, nil, &d, nil, nil, nil); err != nil {
			e.t.Fatal(err)
		}
	}
	return c
}

func (e *gateEnv) move(is models.Issue, state, actor string, force bool) error {
	_, err := e.svc.UpdateIssue(e.ctx, e.ws, is.ID, store.IssuePatch{StateName: &state, ForceGate: force, BlockedReason: "waiting on a decision"}, actor)
	return err
}

func (e *gateEnv) stateOf(is models.Issue) string {
	e.t.Helper()
	got, err := e.svc.Store.GetIssue(e.ctx, e.ws, is.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return e.svc.stateName(e.ctx, e.ws, got.StateID)
}

func asGate(t *testing.T, err error) *store.GateError {
	t.Helper()
	var ge *store.GateError
	if !errors.As(err, &ge) {
		t.Fatalf("want *GateError, got %v", err)
	}
	return ge
}

func TestGateOpenCriterionBlocksReviewAndDone(t *testing.T) {
	e := newGateEnv(t)
	for _, target := range []string{"In Review", "Done", "in review"} {
		is := e.issue("In Progress")
		e.crit(is.ID, "first", models.CriterionManual, true)
		e.crit(is.ID, "second", models.CriterionManual, false)
		err := e.move(is, target, "ai", false)
		ge := asGate(t, err)
		if ge.Code != store.GateCriteriaIncomplete {
			t.Fatalf("%s: code %s", target, ge.Code)
		}
		if len(ge.Open) != 1 || ge.Open[0].Index != 2 || ge.Open[0].Text != "second" {
			t.Fatalf("%s: open = %+v", target, ge.Open)
		}
		if !strings.Contains(err.Error(), "criteria_incomplete") || !strings.Contains(err.Error(), "2. second") {
			t.Fatalf("message = %q", err)
		}
		if got := e.stateOf(is); got != "In Progress" {
			t.Fatalf("state changed to %s despite refusal", got)
		}
	}
}

func TestGateAllTickedAllows(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("In Progress")
	e.crit(is.ID, "a", models.CriterionManual, true)
	e.crit(is.ID, "b", models.CriterionManual, true)
	if err := e.move(is, "In Review", "ai", false); err != nil {
		t.Fatal(err)
	}
	if err := e.move(is, "Done", "ai", false); err != nil {
		t.Fatal(err)
	}
}

func TestGateNoCriteriaIsMissing(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("In Progress")
	if ge := asGate(t, e.move(is, "Done", "human", false)); ge.Code != store.GateCriteriaMissing {
		t.Fatalf("code %s", ge.Code)
	}
}

func TestGateOtherStatesNotGated(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("Backlog")
	e.crit(is.ID, "open", models.CriterionManual, false)
	for _, st := range []string{"Ready", "In Progress", "Blocked", "Canceled", "Backlog"} {
		if err := e.move(is, st, "ai", false); err != nil {
			t.Fatalf("%s: %v", st, err)
		}
	}
	bare := e.issue("Backlog")
	if err := e.move(bare, "Canceled", "ai", false); err != nil {
		t.Fatal(err)
	}
}

func TestGateJudgmentIsAdvisory(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("In Progress")
	e.crit(is.ID, "manual", models.CriterionManual, true)
	e.crit(is.ID, "model says ok", models.CriterionJudgment, false)
	if err := e.move(is, "Done", "ai", false); err != nil {
		t.Fatalf("open judgment item blocked the move: %v", err)
	}
	// An open non-judgment item still blocks beside it.
	is2 := e.issue("In Progress")
	e.crit(is2.ID, "model says ok", models.CriterionJudgment, false)
	e.crit(is2.ID, "manual open", models.CriterionManual, false)
	ge := asGate(t, e.move(is2, "Done", "ai", false))
	if len(ge.Open) != 1 || ge.Open[0].Text != "manual open" || ge.Open[0].Index != 2 {
		t.Fatalf("open = %+v", ge.Open)
	}
}

func TestGateForceByHumanLogsOverride(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("In Progress")
	e.crit(is.ID, "open", models.CriterionManual, false)
	if err := e.move(is, "Done", "human", true); err != nil {
		t.Fatal(err)
	}
	if got := e.stateOf(is); got != "Done" {
		t.Fatalf("state %s", got)
	}
	acts, err := e.svc.Store.ListActivity(e.ctx, e.ws, is.ID)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range acts {
		if a.Kind == "gate_overridden" {
			n++
			if !strings.Contains(a.Detail, "criteria_incomplete") {
				t.Fatalf("detail = %q", a.Detail)
			}
		}
	}
	if n != 1 {
		t.Fatalf("gate_overridden rows = %d", n)
	}
}

func TestGateAIForceIgnored(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("In Progress")
	e.crit(is.ID, "open", models.CriterionManual, false)
	asGate(t, e.move(is, "Done", "ai", true))
	if got := e.stateOf(is); got != "In Progress" {
		t.Fatalf("state %s", got)
	}
}

func TestGateForceNotLoggedWhenNotNeeded(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("In Progress")
	e.crit(is.ID, "ok", models.CriterionManual, true)
	if err := e.move(is, "Done", "human", true); err != nil {
		t.Fatal(err)
	}
	acts, _ := e.svc.Store.ListActivity(e.ctx, e.ws, is.ID)
	for _, a := range acts {
		if a.Kind == "gate_overridden" {
			t.Fatal("override logged for a satisfied gate")
		}
	}
}

func TestGateCreateDirectlyInGatedState(t *testing.T) {
	e := newGateEnv(t)
	_, err := e.svc.CreateIssue(e.ctx, e.ws, store.IssueInput{Title: "x", StateName: "Done"}, "ai")
	if ge := asGate(t, err); ge.Code != store.GateCriteriaMissing {
		t.Fatalf("code %s", ge.Code)
	}
	// AI force is ignored; an admin session may force.
	_, err = e.svc.CreateIssue(e.ctx, e.ws, store.IssueInput{Title: "x", StateName: "Done", ForceGate: true}, "ai")
	asGate(t, err)
	if _, err := e.svc.CreateIssue(e.ctx, e.ws, store.IssueInput{Title: "x", StateName: "Done", ForceGate: true}, "human"); err != nil {
		t.Fatal(err)
	}
}

func TestGateUntickOnDoneIssueAllowed(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("In Progress")
	c := e.crit(is.ID, "a", models.CriterionManual, true)
	if err := e.move(is, "Done", "ai", false); err != nil {
		t.Fatal(err)
	}
	d := false
	if _, err := e.svc.Store.UpdateCriterion(e.ctx, e.ws, c.ID, nil, &d, nil, nil, nil); err != nil {
		t.Fatalf("unticking on a Done issue refused: %v", err)
	}
	if got := e.stateOf(is); got != "Done" {
		t.Fatalf("state %s", got)
	}
	// Re-writing the same state is not a move, so it is not gated.
	if err := e.move(is, "Done", "ai", false); err != nil {
		t.Fatalf("no-op state write gated: %v", err)
	}
}
