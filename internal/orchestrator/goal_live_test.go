package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestGoalLive runs a real harness headless through the goal loop: the check
// fails once on purpose, so the harness's own session must be resumed with
// the failure and fix it.
//
//	RAENIL_GOAL_LIVE=claude|codex|opencode|agy go test ./internal/orchestrator -run TestGoalLive -v
func TestGoalLive(t *testing.T) {
	harness := os.Getenv("RAENIL_GOAL_LIVE")
	if harness == "" {
		t.Skip("set RAENIL_GOAL_LIVE=claude|codex|opencode|agy to run a real harness")
	}
	var runner Runner
	model := os.Getenv("RAENIL_GOAL_MODEL")
	switch harness {
	case "claude":
		r := &ClaudeRunner{}
		conns := DefaultConnections()
		if tok, ok := conns.ClaudeToken(); ok {
			r.OAuthToken, r.ConfigDir = tok, conns.ClaudeConfigDir()
		}
		runner = r
	case "codex":
		runner = &CodexRunner{}
	case "opencode":
		url := os.Getenv("OPENCODE_URL")
		if url == "" {
			t.Skip("OPENCODE_URL is not set")
		}
		runner = &OpenCodeRunner{BaseURL: url}
	case "agy":
		runner = &AntigravityRunner{}
	}

	root := t.TempDir()
	wt := filepath.Join(root, "worktrees", "LIVE-1-1")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", wt, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	checks := 0
	check := func(ctx context.Context) CheckResult {
		checks++
		b, err := os.ReadFile(filepath.Join(wt, "hello.txt"))
		switch {
		case err != nil:
			return CheckResult{Feedback: "Raenil ran the checks. hello.txt does not exist. Create it.", State: "missing"}
		case strings.TrimSpace(string(b)) != "hello world":
			return CheckResult{Feedback: "Raenil ran the checks. hello.txt must contain exactly `hello world`, not `" +
				strings.TrimSpace(string(b)) + "`. Fix it.", State: string(b)}
		}
		return CheckResult{Pass: true}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req := RunRequest{
		Prompt:  "Create a file named hello.txt in the current directory containing exactly the word: hello\nDo nothing else, and do not ask questions.",
		Cwd:     wt,
		Model:   model,
		Timeout: 5 * time.Minute,
		LogPath: filepath.Join(root, "run", "worker.log"),
	}
	o := &Orchestrator{Log: t.Logf}
	first, err := runner.Run(ctx, req)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := o.goalLoop(ctx, runner, req, first, nil, check, "")
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	t.Logf("result: session=%s tokens=%d cost=%.4f answer=%q", res.SessionID, res.Tokens, res.CostUSD, res.Answer)
	if checks < 2 {
		t.Errorf("checks ran %d times; the first must fail and be resumed", checks)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "hello.txt")); strings.TrimSpace(string(b)) != "hello world" {
		t.Errorf("hello.txt = %q after the loop", b)
	}
}
