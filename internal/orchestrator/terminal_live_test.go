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

// TestTerminalLive runs a real harness in a real terminal session: the check
// fails once on purpose, so the failure must be typed back in and fixed.
//
//	RAENIL_TERMINAL_LIVE=claude|codex|opencode go test ./internal/orchestrator -run TestTerminalLive -v
func TestTerminalLive(t *testing.T) {
	harness := os.Getenv("RAENIL_TERMINAL_LIVE")
	if harness == "" {
		t.Skip("set RAENIL_TERMINAL_LIVE=claude|codex|opencode to run a real terminal session")
	}
	var runner Runner
	model := ""
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
		model = os.Getenv("RAENIL_TERMINAL_MODEL")
	}

	// The worktree lives under a folder of its own, as the host's do.
	root := t.TempDir()
	wt := filepath.Join(root, "worktrees", "LIVE-1-1")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", wt, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}

	checks := 0
	check := func(ctx context.Context) (bool, string) {
		checks++
		b, err := os.ReadFile(filepath.Join(wt, "hello.txt"))
		switch {
		case err != nil:
			return false, "Raenil ran the checks. hello.txt does not exist. Create it, then finish your turn."
		case strings.TrimSpace(string(b)) != "hello world":
			return false, "Raenil ran the checks. hello.txt must contain exactly `hello world`, not `" +
				strings.TrimSpace(string(b)) + "`. Fix it, then finish your turn."
		}
		return true, ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	res, err := runner.Run(ctx, RunRequest{
		Prompt:   "Create a file named hello.txt in the current directory containing exactly the word: hello\nDo nothing else, and do not ask questions.",
		Cwd:      wt,
		Model:    model,
		LogPath:  filepath.Join(root, "run", "worker.log"),
		Terminal: true,
		Title:    "LIVE-" + harness,
		Check:    check,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	log, _ := os.ReadFile(filepath.Join(root, "run", "worker.log"))
	t.Logf("result: %+v\nlog:\n%s", res, log)
	if checks < 2 {
		t.Errorf("checks ran %d times; the first must fail and be typed back", checks)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "hello.txt")); strings.TrimSpace(string(b)) != "hello world" {
		t.Errorf("hello.txt = %q after the loop", b)
	}
	if exec.Command("tmux", "has-session", "-t", sessionName("LIVE-"+harness)).Run() == nil {
		t.Errorf("the tmux session was left running")
	}
}
