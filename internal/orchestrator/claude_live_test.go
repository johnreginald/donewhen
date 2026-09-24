package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClaudeLive runs the real CLI on the logged-in subscription. It spends a
// few cents of plan usage, so it only runs when asked.
func TestClaudeLive(t *testing.T) {
	if os.Getenv("RAENIL_CLAUDE_LIVE") == "" {
		t.Skip("set RAENIL_CLAUDE_LIVE=1 to run against the real claude CLI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	r := &ClaudeRunner{}
	const model = "claude/haiku"

	if err := r.Ready(ctx, model); err != nil {
		t.Fatalf("ready: %v", err)
	}

	dir := t.TempDir()
	outside := filepath.Join(os.TempDir(), "raenil-claude-live-outside.txt")
	os.Remove(outside)
	defer os.Remove(outside)

	res, err := r.Run(ctx, RunRequest{
		Cwd:   dir,
		Model: model,
		Prompt: "Do both steps, then reply done.\n" +
			"1. Create a file named inside.txt in the current directory containing the word hello.\n" +
			"2. Use the Bash tool to run exactly: touch " + outside,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("work run: exit=%d session=%s tokens=%d usage=%+v notional=$%.4f denied=%v",
		res.Exit, res.SessionID, res.Tokens, res.Usage, res.NotionalCostUSD, res.DeniedTools)
	if b, err := os.ReadFile(filepath.Join(dir, "inside.txt")); err != nil || !strings.Contains(string(b), "hello") {
		t.Errorf("the worker could not write inside its worktree: %v", err)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Errorf("the worker wrote outside its worktree: %s exists", outside)
	}
	if len(res.DeniedTools) == 0 {
		t.Error("the refused write was not reported in DeniedTools")
	}
	if res.CostUSD != 0 || res.NotionalCostUSD == 0 {
		t.Errorf("subscription run billed: cost=%v notional=%v", res.CostUSD, res.NotionalCostUSD)
	}

	// A reviewer can read the worktree and cannot change it.
	answer, _, err := r.AskIn(ctx, model, "What word is in inside.txt? Reply with just the word. Do not modify any file.", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(answer), "hello") {
		t.Errorf("reviewer answer = %q, want hello", answer)
	}
}
