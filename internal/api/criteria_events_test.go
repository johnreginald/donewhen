package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
)

// Board cards read criteria counts from live events, so every criterion
// change must publish issue.updated with the new counts.
func TestCriterionChangesPublishIssueUpdated(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	ws := e.workspace(owner)
	opt := browser(e.session(owner))

	w := e.do("POST", "/api/issues", map[string]any{"title": "crit", "stateName": "Backlog"}, opt)
	var is models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &is)

	sub := e.srv.bus.Subscribe(ws.ID)
	t.Cleanup(sub.Close)
	expect := func(step string, done, total int) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case ev := <-sub.Events:
				if ev.Type != events.IssueUpdated || ev.Issue == nil || ev.Issue.ID != is.ID {
					continue
				}
				if ev.Issue.CriteriaDone != done || ev.Issue.CriteriaTotal != total {
					t.Fatalf("%s: got %d/%d, want %d/%d", step, ev.Issue.CriteriaDone, ev.Issue.CriteriaTotal, done, total)
				}
				return
			case <-deadline:
				t.Fatalf("%s: no issue.updated event", step)
			}
		}
	}

	w = e.do("POST", "/api/issues/"+is.ID+"/criteria", map[string]any{"body": "one"}, opt)
	if w.Code != 201 {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	var c models.Criterion
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	expect("add", 0, 1)

	if w = e.do("PATCH", "/api/criteria/"+c.ID, map[string]any{"done": true}, opt); w.Code != 200 {
		t.Fatalf("tick: %d %s", w.Code, w.Body)
	}
	expect("tick", 1, 1)

	if w = e.do("PATCH", "/api/criteria/"+c.ID, map[string]any{"done": false}, opt); w.Code != 200 {
		t.Fatalf("untick: %d %s", w.Code, w.Body)
	}
	expect("untick", 0, 1)

	if w = e.do("DELETE", "/api/criteria/"+c.ID, nil, opt); w.Code != 204 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	expect("delete", 0, 0)
}
