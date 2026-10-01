package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-203: save_issue refuses a move into In Review / Done while done-when
// criteria are open, and a token cannot talk its way past it.
func TestSaveIssueDoneWhenGate(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	w := e.workspace(uid)
	ctx := e.ctxFor(uid, w.ID)
	bg := context.Background()

	out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "gated", "state": "In Progress"})
	if isErr {
		t.Fatal(out)
	}
	var is models.Issue
	if err := json.Unmarshal([]byte(out), &is); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		got, err := e.store.GetIssue(bg, w.ID, is.ID)
		if err != nil {
			t.Fatal(err)
		}
		st, _ := e.store.GetState(bg, w.ID, got.StateID)
		return st.Name
	}

	// No criteria at all.
	out, isErr = e.call(ctx, "save_issue", map[string]any{"id": is.Key, "state": "Done"})
	if !isErr || !strings.Contains(out, "criteria_missing") {
		t.Fatalf("no criteria: err=%v %s", isErr, out)
	}

	one, _ := e.store.AddCriterion(bg, w.ID, is.ID, "tests pass", "", nil)
	if _, err := e.store.AddCriterion(bg, w.ID, is.ID, "docs written", "", nil); err != nil {
		t.Fatal(err)
	}
	done := true
	if _, err := e.store.UpdateCriterion(bg, w.ID, one.ID, nil, &done, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	// One unticked: the error names it, the state stays.
	for _, target := range []string{"Done", "In Review"} {
		out, isErr = e.call(ctx, "save_issue", map[string]any{"id": is.Key, "state": target})
		if !isErr || !strings.Contains(out, "criteria_incomplete") || !strings.Contains(out, "2. docs written") {
			t.Fatalf("%s: err=%v %s", target, isErr, out)
		}
		if strings.Contains(out, "tests pass") {
			t.Fatalf("ticked item listed as open: %s", out)
		}
	}
	if got := state(); got != "In Progress" {
		t.Fatalf("state moved to %s", got)
	}

	// There is no force argument for a token: passing one changes nothing.
	out, isErr = e.call(ctx, "save_issue", map[string]any{"id": is.Key, "state": "Done", "force": true})
	if !isErr || !strings.Contains(out, "criteria_incomplete") {
		t.Fatalf("force honoured: err=%v %s", isErr, out)
	}

	// Creating straight into Done is refused too.
	out, isErr = e.call(ctx, "save_issue", map[string]any{"title": "x", "state": "Done"})
	if !isErr || !strings.Contains(out, "criteria_missing") {
		t.Fatalf("create in Done: err=%v %s", isErr, out)
	}

	// Other states are never gated.
	if out, isErr = e.call(ctx, "save_issue", map[string]any{"id": is.Key, "state": "Blocked"}); isErr {
		t.Fatalf("blocked: %s", out)
	}

	// All ticked: the move goes through.
	rest, _ := e.store.ListCriteria(bg, w.ID, is.ID)
	for _, c := range rest {
		if _, err := e.store.UpdateCriterion(bg, w.ID, c.ID, nil, &done, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if out, isErr = e.call(ctx, "save_issue", map[string]any{"id": is.Key, "state": "In Review"}); isErr {
		t.Fatalf("ticked move: %s", out)
	}
	if got := state(); got != "In Review" {
		t.Fatalf("state %s", got)
	}
}
