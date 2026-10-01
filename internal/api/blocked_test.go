package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-207: a move to Blocked needs a reason, which is kept as a comment and on
// the activity row, and GET /api/blocked lists the issue with it.

func TestBlockedNeedsReasonAndIsListed(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	e.workspace(owner)
	opt := browser(e.session(owner))

	var is models.Issue
	w := e.do("POST", "/api/issues", map[string]any{"title": "stuck", "stateName": "In Progress"}, opt)
	_ = json.Unmarshal(w.Body.Bytes(), &is)
	var dep models.Issue
	w = e.do("POST", "/api/issues", map[string]any{"title": "dep", "stateName": "Ready"}, opt)
	_ = json.Unmarshal(w.Body.Bytes(), &dep)
	if w := e.do("PUT", "/api/issues/"+is.ID+"/blockers", map[string]any{"blockedBy": []string{dep.Key}}, opt); w.Code != 200 {
		t.Fatalf("set blockers: %d %s", w.Code, w.Body.String())
	}

	// No reason, blank reason, too long: 400 reason_required, nothing moves.
	for name, body := range map[string]map[string]any{
		"missing": {"stateName": "Blocked"},
		"blank":   {"stateName": "Blocked", "blockedReason": "   "},
		"long":    {"stateName": "Blocked", "blockedReason": strings.Repeat("x", 501)},
	} {
		w := e.do("PATCH", "/api/issues/"+is.ID, body, opt)
		if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"reason_required"`) {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	// The move endpoint (board drops) enforces it too.
	if w := e.do("POST", "/api/issues/"+is.ID+"/move", map[string]any{"state": "Blocked"}, opt); w.Code != 400 {
		t.Fatalf("move without reason: %d", w.Code)
	}
	if w := e.do("GET", "/api/blocked", nil, opt); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("nothing should be blocked yet: %s", w.Body.String())
	}

	// With a reason: moved, comment of kind blocked_reason, reason on the activity row.
	if w := e.do("PATCH", "/api/issues/"+is.ID, map[string]any{"stateName": "Blocked", "blockedReason": "  needs the API key  "}, opt); w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	var comments []models.Comment
	_ = json.Unmarshal(e.do("GET", "/api/issues/"+is.ID+"/comments", nil, opt).Body.Bytes(), &comments)
	if len(comments) != 1 || comments[0].Kind != "blocked_reason" || comments[0].BodyMD != "needs the API key" {
		t.Fatalf("comments = %+v", comments)
	}
	var detail string
	if err := e.pool.QueryRow(t.Context(),
		`SELECT detail FROM activity WHERE issue_id=$1 AND kind='state_changed' AND to_val='Blocked'`, is.ID).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if detail != "needs the API key" {
		t.Fatalf("activity detail = %q", detail)
	}

	var list []models.BlockedItem
	w = e.do("GET", "/api/blocked", nil, opt)
	if w.Code != 200 {
		t.Fatalf("blocked: %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Key != is.Key || list[0].Reason != "needs the API key" ||
		list[0].Actor != "human" || len(list[0].WaitingOn) != 1 || list[0].WaitingOn[0] != dep.Key || list[0].Since.IsZero() {
		t.Fatalf("blocked list = %+v", list)
	}

	// Moving back needs no reason, and it leaves the list.
	if w := e.do("PATCH", "/api/issues/"+is.ID, map[string]any{"stateName": "In Progress"}, opt); w.Code != 200 {
		t.Fatalf("unblock: %d", w.Code)
	}
	if w := e.do("GET", "/api/blocked", nil, opt); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("still listed: %s", w.Body.String())
	}
}
