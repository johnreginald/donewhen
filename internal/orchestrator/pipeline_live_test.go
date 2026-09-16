package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"raenil/internal/models"
)

// TestPipelineLive is the whole thing end to end against a real agent: a repo
// with a failing test, an isolated worktree, a real model doing the work, and
// the criteria engine deciding from evidence whether it counts.
//
//	opencode serve --port 4096 &
//	OPENCODE_URL=http://127.0.0.1:4096 OPENCODE_MODEL=opencode-go/glm-5.3-flash \
//	  go test ./internal/orchestrator/ -run PipelineLive -v -timeout 10m
func TestPipelineLive(t *testing.T) {
	base, model := os.Getenv("OPENCODE_URL"), os.Getenv("OPENCODE_MODEL")
	if base == "" || model == "" {
		t.Skip("set OPENCODE_URL and OPENCODE_MODEL to run the live pipeline")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 needed for the sample project")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	// A repo whose test fails because the implementation is wrong.
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")
	gitRun(t, repo, "config", "user.email", "t@example.com")
	gitRun(t, repo, "config", "user.name", "Test")
	// A real repo declares what is not source. Without this, the bytecode pytest
	// writes lands in the diff and trips paths_within — the orchestrator
	// deliberately has no ignore list of its own, because silently dropping
	// paths from a diff is how a gate quietly stops gating.
	writeFile(t, repo, ".gitignore", "__pycache__/\n*.pyc\n")
	writeFile(t, repo, "add.py", "def add(a, b):\n    return a - b\n")
	writeFile(t, repo, "test_add.py", "from add import add\n\n\ndef test_add():\n    assert add(2, 3) == 5\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "init")

	wtPath := filepath.Join(t.TempDir(), "wt")
	wt, err := AddWorktree(ctx, repo, wtPath, "ticket/LIVE-1", "HEAD")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	defer wt.Remove(context.Background())

	runDir, err := NewRunDir(t.TempDir(), "LIVE-1", 1)
	if err != nil {
		t.Fatal(err)
	}

	runner := &OpenCodeRunner{BaseURL: base, PollInterval: time.Second}
	res, err := runner.Run(ctx, RunRequest{
		Prompt: "The test in test_add.py fails. Fix the bug in add.py so it passes. " +
			"Do not modify test_add.py. Do not create any other files.",
		Cwd:     wtPath,
		Model:   model,
		LogPath: runDir.File("worker.log"),
		Timeout: 6 * time.Minute,
	})
	if err != nil {
		t.Fatalf("runner: %v", err)
	}
	t.Logf("worker: exit=%d cost=$%.4f tokens=%d duration=%s",
		res.Exit, res.CostUSD, res.Tokens, res.Duration.Round(time.Second))

	diff, err := StageAndDiff(ctx, wtPath, "HEAD")
	if err != nil {
		t.Fatalf("StageAndDiff: %v", err)
	}
	for _, f := range diff.Files {
		t.Logf("diff: %s %s (+%d/-%d)", f.Status, f.Path, f.Insertions, f.Deletions)
	}

	criteria, err := ParseCriteria([]models.Criterion{
		{Body: "pytest passes", Kind: models.CriterionDeterministic,
			CheckSpec: json.RawMessage(`{"cmd":"python3 -m pytest -q","timeout":"2m"}`)},
		{Body: "only add.py touched", Kind: models.CriterionPolicy,
			CheckSpec: json.RawMessage(`{"policy":"paths_within","args":["add.py"]}`)},
		{Body: "tests not weakened", Kind: models.CriterionPolicy,
			CheckSpec: json.RawMessage(`{"policy":"tests_not_weakened"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}

	ev, err := (&Evaluator{WorkDir: wtPath, Dir: runDir}).Evaluate(ctx, criteria, diff)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	for _, e := range ev {
		t.Logf("criterion %d %q: pass=%v %s%s", e.CriterionIndex+1, e.CriterionText, e.Pass, e.Detail, e.Err)
	}

	v := Summarise("LIVE-1", 1, criteria, ev, diff)
	v.Runner, v.Model, v.CostUSD = runner.Name(), model, res.CostUSD
	v.DurationS = int64(res.Duration.Seconds())
	if err := runDir.WriteVerdict(v); err != nil {
		t.Fatal(err)
	}
	t.Logf("verdict: status=%s next=%s passed=%v failed=%v", v.Status, v.Next, v.Passed, v.Failed)

	if v.Status != StatusPassed {
		t.Fatalf("pipeline did not reach a passing verdict: %+v", v)
	}
	// Evidence must be on disk and point at a hashed log.
	if _, err := os.Stat(runDir.File("evidence.json")); err != nil {
		t.Errorf("evidence.json missing: %v", err)
	}
	if ev[0].OutputSHA256 == "" {
		t.Error("the deterministic criterion should carry a hashed log")
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
