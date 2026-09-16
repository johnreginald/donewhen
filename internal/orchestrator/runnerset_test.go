package orchestrator

import (
	"context"
	"strings"
	"testing"

	"raenil/internal/models"
)

type namedRunner struct{ name string }

func (r namedRunner) Name() string { return r.name }
func (r namedRunner) Run(_ context.Context, _ RunRequest) (RunResult, error) {
	return RunResult{}, nil
}

func TestRunnerForPicksByLabel(t *testing.T) {
	set := RunnerSet{"opencode": namedRunner{"opencode"}, "codex": namedRunner{"codex"}}
	fallback := namedRunner{"opencode"}

	issue := models.Issue{Key: "T-1", Labels: []models.Label{{Name: "repo:web"}, {Name: "runner:codex"}}}
	r, err := set.RunnerFor(issue, fallback)
	if err != nil || r.Name() != "codex" {
		t.Errorf("label should select codex, got %v %v", r, err)
	}

	// Case should not matter; people type labels by hand.
	issue.Labels = []models.Label{{Name: "Runner:CODEX"}}
	if r, err := set.RunnerFor(issue, fallback); err != nil || r.Name() != "codex" {
		t.Errorf("label match should be case-insensitive, got %v %v", r, err)
	}

	// No label means the configured default.
	if r, err := set.RunnerFor(models.Issue{Key: "T-2"}, fallback); err != nil || r.Name() != "opencode" {
		t.Errorf("unlabelled ticket should use the fallback, got %v %v", r, err)
	}
}

// A ticket naming a runner that is not configured must fail loudly. Quietly
// running the work on a different agent than the one asked for is a
// substitution nobody notices until the results look strange.
func TestRunnerForRefusesUnknownRunner(t *testing.T) {
	set := RunnerSet{"opencode": namedRunner{"opencode"}}
	issue := models.Issue{Key: "T-1", Labels: []models.Label{{Name: "runner:codex"}}}

	_, err := set.RunnerFor(issue, namedRunner{"opencode"})
	if err == nil {
		t.Fatal("expected a refusal, not a silent fallback")
	}
	if !strings.Contains(err.Error(), "codex") || !strings.Contains(err.Error(), "opencode") {
		t.Errorf("error should name what was asked for and what is available: %v", err)
	}
}
