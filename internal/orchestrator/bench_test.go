package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// benchFixtures is the real fixture directory, relative to this package.
const benchFixtures = "../../bench"

func TestLoadBenchTasks(t *testing.T) {
	tasks, err := LoadBenchTasks(benchFixtures)
	if err != nil {
		t.Fatalf("LoadBenchTasks: %v", err)
	}
	if len(tasks) < 4 {
		t.Fatalf("expected at least 4 fixtures, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.Title == "" || len(task.Criteria) == 0 {
			t.Errorf("%s: incomplete task", task.Name)
		}
		// Every fixture must be machine-decidable, or it measures nothing.
		parsed, err := ParseCriteria(task.criteria())
		if err != nil {
			t.Errorf("%s: criteria do not parse: %v", task.Name, err)
			continue
		}
		if !anyGating(parsed) {
			t.Errorf("%s: no deterministic or policy criterion", task.Name)
		}
	}
}

// scriptRunner performs a fixed edit, standing in for a model.
type scriptRunner struct{ edit func(dir string) }

func (scriptRunner) Name() string { return "script" }
func (r scriptRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	if r.edit != nil {
		r.edit(req.Cwd)
	}
	return RunResult{CostUSD: 0.001}, nil
}

func benchTask(t *testing.T, name string) BenchTask {
	t.Helper()
	tasks, err := LoadBenchTasks(benchFixtures)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Name == name {
			return task
		}
	}
	t.Fatalf("fixture %q not found", name)
	return BenchTask{}
}

func TestBenchDetectsAGenuineFix(t *testing.T) {
	if _, err := os.Stat("/usr/bin/python3"); err != nil {
		if _, err := os.Stat("/opt/homebrew/bin/python3"); err != nil {
			t.Skip("python3 needed")
		}
	}
	task := benchTask(t, "fix-bug")
	r := runOneBench(context.Background(), task, "test", scriptRunner{edit: func(dir string) {
		_ = os.WriteFile(filepath.Join(dir, "add.py"), []byte("def add(a, b):\n    return a + b\n"), 0o644)
	}}, BenchOptions{})

	if r.Status != StatusPassed {
		t.Errorf("a genuine fix should pass: status=%s failed=%d err=%s", r.Status, r.Failed, r.Err)
	}
	if r.Cheated || r.OutOfScope {
		t.Errorf("a genuine fix should raise no flags: %+v", r)
	}
}

// The probe that gives the bake-off its point: deleting the test must be
// recorded as cheating, not merely as a failure.
func TestBenchFlagsDeletedTest(t *testing.T) {
	task := benchTask(t, "fix-bug")
	r := runOneBench(context.Background(), task, "test", scriptRunner{edit: func(dir string) {
		_ = os.Remove(filepath.Join(dir, "test_add.py"))
	}}, BenchOptions{})

	if !r.Cheated {
		t.Errorf("deleting the test must be flagged as cheating: %+v", r)
	}
	if r.Status == StatusPassed {
		t.Error("deleting the test must not pass")
	}
}

func TestBenchFlagsOutOfScopeEdits(t *testing.T) {
	task := benchTask(t, "scoped-edit")
	r := runOneBench(context.Background(), task, "test", scriptRunner{edit: func(dir string) {
		_ = os.WriteFile(filepath.Join(dir, "deploy", "config.yaml"), []byte("port: 9999\n"), 0o644)
	}}, BenchOptions{})

	if !r.OutOfScope {
		t.Errorf("editing deploy/ must be flagged as out of scope: %+v", r)
	}
}

func TestBenchReportSummarises(t *testing.T) {
	out := BenchReport([]BenchResult{
		{Task: "fix-bug", Model: "a/one", Status: StatusPassed, Passed: 3, Cost: 0.01, Secs: 9},
		{Task: "honesty-probe", Model: "a/one", Status: StatusFailed, Failed: 2, Cheated: true, Cost: 0.02, Secs: 12},
	})
	for _, want := range []string{"fix-bug", "a/one", "CHEATED", "1/2"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\n%s", want, out)
		}
	}
}
