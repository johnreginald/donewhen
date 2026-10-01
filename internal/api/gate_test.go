package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"raenil/internal/config"
	"raenil/internal/models"
)

// PP-203: REST refuses a move into In Review / Done while done-when criteria
// are open, with a stable code and the open items; only an owner/admin browser
// session may force it.

type gateResp struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	State string `json:"state"`
	Open  []struct {
		Index int    `json:"index"`
		Text  string `json:"text"`
	} `json:"open"`
}

func gateIssue(t *testing.T, e *testEnv, opt reqOpt, crit ...string) string {
	t.Helper()
	w := e.do("POST", "/api/issues", map[string]any{"title": "gated", "stateName": "In Progress"}, opt)
	if w.Code != http.StatusCreated {
		t.Fatalf("create issue: %d %s", w.Code, w.Body.String())
	}
	var is models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &is)
	for _, c := range crit {
		if w := e.do("POST", "/api/issues/"+is.ID+"/criteria", map[string]any{"body": c}, opt); w.Code != http.StatusCreated {
			t.Fatalf("add criterion: %d %s", w.Code, w.Body.String())
		}
	}
	return is.ID
}

func TestGateREST(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	wsp := e.workspace(owner)
	member, _ := e.user("")
	if err := e.store.AddMember(t.Context(), wsp.ID, member, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	ownerBrowser := browser(e.session(owner))
	memberBrowser := browser(e.session(member))
	tok := bearer(e.token(owner, wsp.ID))

	id := gateIssue(t, e, ownerBrowser, "first", "second")

	// Open criteria: 409 with code and the list.
	w := e.do("PATCH", "/api/issues/"+id, map[string]any{"stateName": "Done"}, ownerBrowser)
	if w.Code != 409 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var g gateResp
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	if g.Code != "criteria_incomplete" || g.State != "Done" || len(g.Open) != 2 ||
		g.Open[0].Index != 1 || g.Open[1].Text != "second" || g.Error == "" {
		t.Fatalf("body = %s", w.Body.String())
	}

	// Bearer force is ignored; a plain member session cannot force either.
	for name, opt := range map[string]reqOpt{"bearer": tok, "member": memberBrowser} {
		w = e.do("PATCH", "/api/issues/"+id, map[string]any{"stateName": "In Review", "force": true}, opt)
		if w.Code != 409 {
			t.Fatalf("%s force: %d %s", name, w.Code, w.Body.String())
		}
	}

	// Owner session forces: moves, and the override is on the timeline.
	w = e.do("PATCH", "/api/issues/"+id, map[string]any{"stateName": "Done", "force": true}, ownerBrowser)
	if w.Code != 200 {
		t.Fatalf("owner force: %d %s", w.Code, w.Body.String())
	}
	w = e.do("GET", "/api/issues/"+id+"/activity", nil, ownerBrowser)
	var acts []models.Activity
	_ = json.Unmarshal(w.Body.Bytes(), &acts)
	found := false
	for _, a := range acts {
		found = found || a.Kind == "gate_overridden"
	}
	if !found {
		t.Fatalf("no gate_overridden row: %s", w.Body.String())
	}

	// No criteria at all.
	bare := gateIssue(t, e, ownerBrowser)
	w = e.do("PATCH", "/api/issues/"+bare, map[string]any{"stateName": "In Review"}, tok)
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	if w.Code != 409 || g.Code != "criteria_missing" {
		t.Fatalf("no criteria: %d %s", w.Code, w.Body.String())
	}

	// A non-gated move is untouched; all ticked allows the gated one.
	full := gateIssue(t, e, ownerBrowser, "only")
	if w = e.do("PATCH", "/api/issues/"+full, map[string]any{"stateName": "Ready"}, tok); w.Code != 200 {
		t.Fatalf("ready: %d", w.Code)
	}
	w = e.do("GET", "/api/issues/"+full+"/criteria", nil, ownerBrowser)
	var crit []models.Criterion
	_ = json.Unmarshal(w.Body.Bytes(), &crit)
	if w = e.do("PATCH", "/api/criteria/"+crit[0].ID, map[string]any{"done": true}, ownerBrowser); w.Code != 200 {
		t.Fatalf("tick: %d %s", w.Code, w.Body.String())
	}
	if w = e.do("PATCH", "/api/issues/"+full, map[string]any{"stateName": "In Review"}, tok); w.Code != 200 {
		t.Fatalf("ticked move: %d %s", w.Code, w.Body.String())
	}

	// Creating straight into Done is refused for everyone but a forcing admin.
	w = e.do("POST", "/api/issues", map[string]any{"title": "x", "stateName": "Done"}, tok)
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	if w.Code != 409 || g.Code != "criteria_missing" {
		t.Fatalf("create in Done: %d %s", w.Code, w.Body.String())
	}
	if w = e.do("POST", "/api/issues", map[string]any{"title": "x", "stateName": "Done", "force": true}, ownerBrowser); w.Code != 201 {
		t.Fatalf("forced create: %d %s", w.Code, w.Body.String())
	}
}
