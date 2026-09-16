package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"raenil/internal/models"
)

func parse(t *testing.T, in []models.Criterion) []ParsedCriterion {
	t.Helper()
	p, err := ParseCriteria(in)
	if err != nil {
		t.Fatalf("ParseCriteria: %v", err)
	}
	return p
}

func newEval(t *testing.T) *Evaluator {
	t.Helper()
	root := t.TempDir()
	dir, err := NewRunDir(root, "TST-1", 1)
	if err != nil {
		t.Fatalf("NewRunDir: %v", err)
	}
	return &Evaluator{WorkDir: root, Dir: dir}
}

func TestDeterministicPassAndFail(t *testing.T) {
	e := newEval(t)
	cs := parse(t, []models.Criterion{
		{Body: "true exits 0", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"exit 0"}`)},
		{Body: "false exits 1", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"exit 1"}`)},
		{Body: "expects 3", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"exit 3","expect_exit":3}`)},
	})
	ev, err := e.Evaluate(context.Background(), cs, Diff{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !ev[0].Pass {
		t.Errorf("exit 0 should pass, detail=%q err=%q", ev[0].Detail, ev[0].Err)
	}
	if ev[1].Pass {
		t.Error("exit 1 should fail")
	}
	if !ev[2].Pass {
		t.Errorf("exit 3 with expect_exit 3 should pass, detail=%q", ev[2].Detail)
	}
}

func TestDeterministicCapturesOutputAndHash(t *testing.T) {
	e := newEval(t)
	cs := parse(t, []models.Criterion{
		{Body: "echoes", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"echo hello-evidence"}`)},
	})
	ev, _ := e.Evaluate(context.Background(), cs, Diff{})
	if ev[0].OutputSHA256 == "" {
		t.Error("expected an output hash so the log can be proven unedited")
	}
	b, err := os.ReadFile(e.Dir.File(ev[0].OutputPath))
	if err != nil {
		t.Fatalf("read captured log: %v", err)
	}
	if string(b) != "hello-evidence\n" {
		t.Errorf("captured log = %q", b)
	}
}

func TestDeterministicTimeoutIsNotAPass(t *testing.T) {
	e := newEval(t)
	cs := parse(t, []models.Criterion{
		{Body: "hangs", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"sleep 5","timeout":"100ms"}`)},
	})
	ev, _ := e.Evaluate(context.Background(), cs, Diff{})
	if ev[0].Pass {
		t.Error("a timed-out check must never count as a pass")
	}
	if ev[0].Err == "" {
		t.Error("a timed-out check should record why")
	}
}

func TestPolicyCriterionEvaluated(t *testing.T) {
	e := newEval(t)
	cs := parse(t, []models.Criterion{
		{Body: "stays in src", Kind: models.CriterionPolicy, CheckSpec: json.RawMessage(`{"policy":"paths_within","args":["src/**"]}`)},
	})
	ev, _ := e.Evaluate(context.Background(), cs, Diff{Files: []DiffFile{{Path: "src/a.ts"}}})
	if !ev[0].Pass {
		t.Errorf("in-bounds diff should pass, detail=%q", ev[0].Detail)
	}
	ev, _ = e.Evaluate(context.Background(), cs, Diff{Files: []DiffFile{{Path: "deploy/prod.yml"}}})
	if ev[0].Pass {
		t.Error("out-of-bounds diff should fail")
	}
}

// The central guarantee: a model's opinion is advisory. It is reported, and it
// never moves a ticket's status on its own.
func TestJudgmentIsAdvisoryOnly(t *testing.T) {
	e := newEval(t)
	e.Judge = func(ctx context.Context, c JudgmentCheck, d Diff) (bool, string, error) {
		return false, "model dislikes this", nil
	}
	cs := parse(t, []models.Criterion{
		{Body: "build passes", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"exit 0"}`)},
		{Body: "conforms to ADR-42", Kind: models.CriterionJudgment, CheckSpec: json.RawMessage(`{"prompt":"does it?"}`)},
	})
	ev, _ := e.Evaluate(context.Background(), cs, Diff{})
	v := Summarise("TST-1", 1, cs, ev, Diff{})

	if v.Status != StatusPassed {
		t.Errorf("judgment failure must not fail the ticket; status=%s failed=%v", v.Status, v.Failed)
	}
	if len(v.AdvisoryFlagged) != 1 || v.AdvisoryFlagged[0] != 1 {
		t.Errorf("judgment failure should be reported as advisory, got %v", v.AdvisoryFlagged)
	}
}

func TestManualCriterionNeitherPassesNorFails(t *testing.T) {
	e := newEval(t)
	cs := parse(t, []models.Criterion{
		{Body: "someone eyeballs the UI", Kind: models.CriterionManual},
		{Body: "build passes", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"exit 0"}`)},
	})
	ev, _ := e.Evaluate(context.Background(), cs, Diff{})
	v := Summarise("TST-1", 1, cs, ev, Diff{})
	if v.Status != StatusPassed {
		t.Errorf("a manual criterion should not block; status=%s failed=%v", v.Status, v.Failed)
	}
	if len(v.Passed) != 1 {
		t.Errorf("only the deterministic criterion should count as passed, got %v", v.Passed)
	}
}

func TestFailedGateSetsRetry(t *testing.T) {
	e := newEval(t)
	cs := parse(t, []models.Criterion{
		{Body: "build passes", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"exit 1"}`)},
	})
	ev, _ := e.Evaluate(context.Background(), cs, Diff{})
	v := Summarise("TST-1", 1, cs, ev, Diff{})
	if v.Status != StatusFailed || v.Next != "retry" {
		t.Errorf("status=%s next=%s, want failed/retry", v.Status, v.Next)
	}
}

func TestEvidenceFlushedIncrementally(t *testing.T) {
	e := newEval(t)
	cs := parse(t, []models.Criterion{
		{Body: "a", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"exit 0"}`)},
		{Body: "b", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{"cmd":"exit 0"}`)},
	})
	if _, err := e.Evaluate(context.Background(), cs, Diff{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(e.Dir.File("evidence.json"))
	if err != nil {
		t.Fatalf("evidence.json should exist on disk: %v", err)
	}
	var got []Evidence
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("evidence.json is not valid JSON: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 evidence entries persisted, got %d", len(got))
	}
}

func TestParseCriteriaRejectsBadSpecs(t *testing.T) {
	for _, c := range []models.Criterion{
		{Body: "no cmd", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{}`)},
		{Body: "no policy", Kind: models.CriterionPolicy, CheckSpec: json.RawMessage(`{}`)},
		{Body: "bad json", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{`)},
		{Body: "weird kind", Kind: "vibes"},
	} {
		if _, err := ParseCriteria([]models.Criterion{c}); err == nil {
			t.Errorf("expected %q to be rejected, not silently skipped", c.Body)
		}
	}
}
