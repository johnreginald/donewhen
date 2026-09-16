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

func TestRepoForPicksByLabel(t *testing.T) {
	set := RepoSet{"api-mobile": "/p/api-mobile", "api-server": "/p/api-server"}

	issue := models.Issue{Key: "API-1", Labels: []models.Label{{Name: "type:bug"}, {Name: "repo:api-server"}}}
	got, err := set.RepoFor(issue, "/fallback")
	if err != nil || got != "/p/api-server" {
		t.Errorf("label should select api-server, got %q %v", got, err)
	}

	// No label falls back to the single configured repo.
	if got, err := set.RepoFor(models.Issue{Key: "API-2"}, "/fallback"); err != nil || got != "/fallback" {
		t.Errorf("unlabelled should use the fallback, got %q %v", got, err)
	}
}

// Writing to the wrong repository is the worst failure here: the work would be
// committed and verified against a codebase nobody asked for, and pass.
func TestRepoForRefusesUnknownRepo(t *testing.T) {
	set := RepoSet{"api-mobile": "/p/api-mobile"}
	issue := models.Issue{Key: "API-1", Labels: []models.Label{{Name: "repo:api-server"}}}

	if _, err := set.RepoFor(issue, "/fallback"); err == nil {
		t.Fatal("expected a refusal, never a fallback to the wrong repository")
	}
	if _, err := set.RepoFor(models.Issue{Key: "API-2"}, ""); err == nil {
		t.Error("no label and no fallback should also be an error")
	}
}
