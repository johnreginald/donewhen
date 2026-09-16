package store

import (
	"context"
	"encoding/json"
	"testing"

	"raenil/internal/models"
)

// A criterion's evidence belongs to its tick. Un-ticking must drop it, or the
// record shows an unmet criterion still citing a previous run's proof.
func TestUnTickingCriterionClearsEvidence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	issue, err := s.CreateIssue(ctx, ws, IssueInput{Title: "evidence", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	spec := json.RawMessage(`{"cmd":"true","expect_exit":0}`)
	c, err := s.AddCriterion(ctx, ws, issue.ID, "build passes", models.CriterionDeterministic, spec)
	if err != nil {
		t.Fatal(err)
	}

	// Tick it with evidence.
	done, ref := true, "runs/X/attempt-1/evidence.json#0"
	c, err = s.UpdateCriterion(ctx, ws, c.ID, nil, &done, nil, nil, &ref)
	if err != nil {
		t.Fatal(err)
	}
	if c.EvidenceRef == nil || *c.EvidenceRef != ref {
		t.Fatalf("evidence not recorded on tick: %v", c.EvidenceRef)
	}

	// Un-tick it, as a re-sent checklist does.
	notDone := false
	c, err = s.UpdateCriterion(ctx, ws, c.ID, nil, &notDone, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Done {
		t.Error("criterion should be un-ticked")
	}
	if c.EvidenceRef != nil {
		t.Errorf("un-ticking must clear the evidence reference, still %q", *c.EvidenceRef)
	}
}

// Editing a criterion without touching `done` must not lose its evidence.
func TestEditingCriterionKeepsEvidence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	issue, err := s.CreateIssue(ctx, ws, IssueInput{Title: "evidence", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.AddCriterion(ctx, ws, issue.ID, "build passes", models.CriterionManual, nil)
	if err != nil {
		t.Fatal(err)
	}
	done, ref := true, "runs/X/attempt-1/evidence.json#0"
	if _, err = s.UpdateCriterion(ctx, ws, c.ID, nil, &done, nil, nil, &ref); err != nil {
		t.Fatal(err)
	}

	body := "build still passes"
	c, err = s.UpdateCriterion(ctx, ws, c.ID, &body, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.EvidenceRef == nil || *c.EvidenceRef != ref {
		t.Errorf("editing the text should not drop evidence, got %v", c.EvidenceRef)
	}
	if !c.Done {
		t.Error("editing the text should not un-tick it")
	}
}

// Re-ticking with fresh evidence replaces the old reference.
func TestReTickingReplacesEvidence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	issue, err := s.CreateIssue(ctx, ws, IssueInput{Title: "evidence", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.AddCriterion(ctx, ws, issue.ID, "build passes", models.CriterionManual, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := true
	first := "attempt-1/evidence.json#0"
	if _, err = s.UpdateCriterion(ctx, ws, c.ID, nil, &done, nil, nil, &first); err != nil {
		t.Fatal(err)
	}
	second := "attempt-2/evidence.json#0"
	c, err = s.UpdateCriterion(ctx, ws, c.ID, nil, &done, nil, nil, &second)
	if err != nil {
		t.Fatal(err)
	}
	if c.EvidenceRef == nil || *c.EvidenceRef != second {
		t.Errorf("evidence = %v, want the newer reference", c.EvidenceRef)
	}
}
