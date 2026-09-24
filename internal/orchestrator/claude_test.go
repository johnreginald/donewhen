package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseClaudeTranscriptSuccess(t *testing.T) {
	tr, err := parseClaudeTranscript("testdata/claude/ok.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if tr.answer != "ok" || tr.isError || tr.subtype != "success" {
		t.Fatalf("answer=%q isError=%v subtype=%q", tr.answer, tr.isError, tr.subtype)
	}
	if tr.sessionID == "" {
		t.Error("no session id")
	}
	if !tr.subscription() {
		t.Errorf("apiKeySource %q should read as a subscription run", tr.apiKeySource)
	}
	want := TokenUsage{Input: 9, CacheCreation: 11475, Output: 40}
	if tr.usage != want {
		t.Errorf("usage = %+v, want %+v", tr.usage, want)
	}

	var res RunResult
	tr.apply(&res)
	if res.CostUSD != 0 || res.NotionalCostUSD == 0 {
		t.Errorf("a subscription run must record notional cost only: cost=%v notional=%v", res.CostUSD, res.NotionalCostUSD)
	}
	if res.Tokens != want.Total() {
		t.Errorf("tokens = %d, want %d", res.Tokens, want.Total())
	}
}

func TestParseClaudeTranscriptDenied(t *testing.T) {
	tr, err := parseClaudeTranscript("testdata/claude/denied.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.denied) != 1 || !strings.HasPrefix(tr.denied[0], "Bash touch /tmp/raenil-outside.txt") {
		t.Fatalf("denied = %q, want the refused Bash call", tr.denied)
	}
}

func TestClaudeAPIRunIsBilled(t *testing.T) {
	tr := claudeTranscript{apiKeySource: "ANTHROPIC_API_KEY", costUSD: 0.5}
	var res RunResult
	tr.apply(&res)
	if res.CostUSD != 0.5 || res.NotionalCostUSD != 0 {
		t.Errorf("an API-key run is real spend: cost=%v notional=%v", res.CostUSD, res.NotionalCostUSD)
	}
}

func TestClaudeArgs(t *testing.T) {
	r := &ClaudeRunner{AllowedTools: []string{"Bash(go test *)"}, MaxBudgetUSD: 2}
	has := func(args []string, pair ...string) bool {
		for i := 0; i+len(pair) <= len(args); i++ {
			if slices.Equal(args[i:i+len(pair)], pair) {
				return true
			}
		}
		return false
	}

	work := r.args(RunRequest{Model: "claude/sonnet", SessionID: "s1"}, "mcp.json")
	for _, pair := range [][]string{
		{"--setting-sources", "project"},
		{"--strict-mcp-config"},
		{"--mcp-config", "mcp.json"},
		{"--permission-mode", "acceptEdits"},
		{"--model", "sonnet"},
		{"--resume", "s1"},
		{"--max-budget-usd", "2.00"},
	} {
		if !has(work, pair...) {
			t.Errorf("work args lack %v: %v", pair, work)
		}
	}
	if i := slices.Index(work, "--allowedTools"); i < 0 || !strings.Contains(work[i+1], "Bash(go test *)") {
		t.Errorf("configured allowlist not passed: %v", work)
	} else if slices.Contains(strings.Split(work[i+1], ","), "Bash") {
		t.Errorf("bare Bash is pre-approved, which lets a worker write anywhere: %v", work[i+1])
	}

	// A model meant for another runner is not handed to Claude.
	if other := r.args(RunRequest{Model: "opencode-go/glm-5.3-flash"}, "m"); slices.Contains(other, "--model") {
		t.Errorf("foreign model passed on: %v", other)
	}
	if judge := r.args(RunRequest{DisableTools: true}, "m"); !has(judge, "--tools", "") || slices.Contains(judge, "acceptEdits") {
		t.Errorf("judge must run with no tools: %v", judge)
	}
	if review := r.args(RunRequest{ReadOnlyTools: true}, "m"); !has(review, "--tools", "Read,Glob,Grep") || slices.Contains(review, "acceptEdits") {
		t.Errorf("reviewer must be read-only: %v", review)
	}
}

// A fake claude proves the prompt travels on stdin and the log is parsed into
// the result, end to end through Run.
func TestClaudeRunWithFakeBinary(t *testing.T) {
	dir := t.TempDir()
	fixture, _ := filepath.Abs("testdata/claude/ok.jsonl")
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ncat > \"$(dirname \"$0\")/stdin.txt\"\necho \"$@\" > \"$(dirname \"$0\")/args.txt\"\ncat " + fixture + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &ClaudeRunner{Bin: bin}
	res, err := r.Run(context.Background(), RunRequest{Prompt: "fix the bug", Cwd: dir, Model: "claude/haiku"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Exit != 0 || res.AgentError != "" || res.SessionID == "" || res.Tokens == 0 {
		t.Fatalf("result = %+v", res)
	}
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin.txt"))
	if string(stdin) != "fix the bug" {
		t.Errorf("stdin = %q, want the prompt", stdin)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	if strings.Contains(string(args), "fix the bug") {
		t.Errorf("prompt leaked onto the command line: %s", args)
	}
}

// A key in the orchestrator's shell must never reach a subscription CLI.
func TestClaudeRunDropsAPIKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-should-not-leak")
	t.Setenv("RAENIL_KEEP_ME", "kept")
	dir := t.TempDir()
	fixture, _ := filepath.Abs("testdata/claude/ok.jsonl")
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nenv > \"$(dirname \"$0\")/env.txt\"\ncat >/dev/null\ncat " + fixture + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (&ClaudeRunner{Bin: bin}).Run(context.Background(), RunRequest{Prompt: "x", Cwd: dir}); err != nil {
		t.Fatal(err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env.txt"))
	if strings.Contains(string(env), "sk-should-not-leak") {
		t.Error("ANTHROPIC_API_KEY reached the claude process")
	}
	if !strings.Contains(string(env), "RAENIL_KEEP_ME=kept") {
		t.Error("unrelated environment was dropped too")
	}
}
