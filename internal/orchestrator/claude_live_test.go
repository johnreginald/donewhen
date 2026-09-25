package orchestrator

import (
	"context"
	"encoding/json"
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

// TestClaudeLiveIsolated checks a run on a Raenil connection is apart from
// the user's own ~/.claude: no hooks fire, and it runs on the subscription.
// It needs `orchestrator connect claude` to have been run on this Mac.
func TestClaudeLiveIsolated(t *testing.T) {
	if os.Getenv("RAENIL_CLAUDE_LIVE") == "" {
		t.Skip("set RAENIL_CLAUDE_LIVE=1 to run against the real claude CLI")
	}
	conns := DefaultConnections()
	tok, ok := conns.ClaudeToken()
	if !ok {
		t.Skip("no Raenil Claude connection: run `orchestrator connect claude`")
	}
	r := &ClaudeRunner{OAuthToken: tok, ConfigDir: conns.ClaudeConfigDir()}
	log := filepath.Join(t.TempDir(), "run.jsonl")
	res, err := r.Run(context.Background(), RunRequest{Prompt: "Reply with exactly: ok", Cwd: t.TempDir(),
		Model: "claude/haiku", DisableTools: true, LogPath: log})
	if err != nil || res.Exit != 0 {
		t.Fatalf("run: %v %+v", err, res)
	}
	// Nothing of the user's may load: every plugin is Claude Code's own
	// (agents-md, which reads a repo's AGENTS.md through a SessionStart hook,
	// and telemetry), and no hook says anything. The user's own hooks — seven
	// of them on this Mac — would show up as more plugins or non-empty output.
	b, _ := os.ReadFile(log)
	for _, line := range strings.Split(string(b), "\n") {
		var ev struct {
			Subtype string `json:"subtype"`
			Plugins []struct {
				Source string `json:"source"`
			} `json:"plugins"`
			Output string `json:"output"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		for _, p := range ev.Plugins {
			if !strings.HasSuffix(p.Source, "@builtin") {
				t.Errorf("a plugin of the user's loaded: %s", p.Source)
			}
		}
		if strings.HasPrefix(ev.Subtype, "hook_response") && strings.TrimSpace(ev.Output) != "{}" && ev.Output != "" {
			t.Errorf("a hook said something to the agent: %q", ev.Output)
		}
	}
	if res.Billing != "subscription" {
		t.Errorf("billing = %q, want subscription", res.Billing)
	}
}
