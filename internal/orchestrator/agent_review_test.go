package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"raenil/internal/models"
)

func TestParseReviewAnswerVerdictFollowsFindings(t *testing.T) {
	// The reviewer says "changes" but finds nothing blocking: it passes.
	a, err := parseReviewAnswer("Here you go:\n```json\n" +
		`{"verdict":"changes","summary":"ok","findings":[{"severity":"minor","issue":"naming"}],"guide":"# g"}` + "\n```")
	if err != nil {
		t.Fatal(err)
	}
	if a.Verdict != "pass" || a.Guide != "# g" || len(a.Findings) != 1 {
		t.Errorf("answer = %+v, want pass with the guide and one minor finding", a)
	}
	// Says "pass" but a finding blocks: changes. Odd severities count as minor.
	a, err = parseReviewAnswer(`{"verdict":"pass","findings":[{"severity":"Blocking","issue":"bug"},{"severity":"nit","issue":"x"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if a.Verdict != "changes" || a.Findings[0].Severity != "blocking" || a.Findings[1].Severity != "minor" {
		t.Errorf("answer = %+v, want changes, blocking + minor", a)
	}
	if _, err := parseReviewAnswer("no json here"); err == nil {
		t.Error("an answer with no JSON parsed")
	}
}

func TestPickReviewerIsAnotherVendor(t *testing.T) {
	pool := RunnerSet{"claude": &ClaudeRunner{}, "codex": &CodexRunner{}}
	if _, name := PickReviewer("claude", pool); name != "codex" {
		t.Errorf("claude's reviewer = %q, want codex", name)
	}
	if _, name := PickReviewer("codex", pool); name != "claude" {
		t.Errorf("codex's reviewer = %q, want claude", name)
	}
	if _, name := PickReviewer("claude", RunnerSet{"claude": &ClaudeRunner{}}); name != "claude" {
		t.Errorf("alone, the builder reviews itself; got %q", name)
	}
	if r, _ := PickReviewer("claude", RunnerSet{}); r != nil {
		t.Error("an empty pool gave a reviewer")
	}
}

func TestTicketHashChangesWithTheTicket(t *testing.T) {
	is := models.Issue{DescriptionMD: "# T"}
	cs := []models.Criterion{{Body: "tests pass", Kind: "deterministic", CheckSpec: json.RawMessage(`{"cmd":"go test"}`)}}
	h := ticketHash(is, cs)
	cs[0].CheckSpec = json.RawMessage(`{"cmd":"go test ./..."}`)
	if ticketHash(is, cs) == h {
		t.Error("editing a check did not change the hash")
	}
}

func TestReviewFeedbackListsBlockingOnly(t *testing.T) {
	fb := reviewFeedback(models.Review{Reviewer: "codex", Findings: []models.ReviewFinding{
		{Severity: "blocking", File: "a.go", Line: 3, Issue: "nil deref", Fix: "check err"},
		{Severity: "minor", Issue: "naming"},
	}})
	if !strings.Contains(fb, "a.go:3 — nil deref Fix: check err") || strings.Contains(fb, "naming") {
		t.Errorf("feedback = %q", fb)
	}
}
