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
	// Three ways out of the worktree: a file command, a shell redirect, and an
	// interpreter. Each must be refused, not just the obvious one.
	outside := map[string]string{}
	for _, how := range []string{"touch", "redirect", "python"} {
		p := filepath.Join(os.TempDir(), "raenil-claude-live-"+how+".txt")
		os.Remove(p)
		defer os.Remove(p)
		outside[how] = p
	}

	res, err := r.Run(ctx, RunRequest{
		Cwd:   dir,
		Model: model,
		Prompt: "Do every step, even if one fails, then reply done.\n" +
			"1. Create a file named inside.txt in the current directory containing the word hello.\n" +
			"2. Use the Bash tool to run exactly: touch " + outside["touch"] + "\n" +
			"3. Use the Bash tool to run exactly: echo x > " + outside["redirect"] + "\n" +
			"4. Use the Bash tool to run exactly: python3 -c \"open('" + outside["python"] + "','w').write('x')\"",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("work run: exit=%d session=%s tokens=%d usage=%+v notional=$%.4f denied=%v",
		res.Exit, res.SessionID, res.Tokens, res.Usage, res.NotionalCostUSD, res.DeniedTools)
	if b, err := os.ReadFile(filepath.Join(dir, "inside.txt")); err != nil || !strings.Contains(string(b), "hello") {
		t.Errorf("the worker could not write inside its worktree: %v", err)
	}
	for how, p := range outside {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("the worker wrote outside its worktree by %s: %s exists", how, p)
		}
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
