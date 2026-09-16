package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestOpenCodeLive exercises a real `opencode serve`. The fake in
// opencode_test.go is built from the published OpenAPI spec; this proves the
// spec matches the running server.
//
//	opencode serve --port 4096 &
//	OPENCODE_URL=http://127.0.0.1:4096 OPENCODE_MODEL=opencode-go/glm-5.3-flash \
//	  go test ./internal/orchestrator/ -run Live -v
func TestOpenCodeLive(t *testing.T) {
	base := os.Getenv("OPENCODE_URL")
	if base == "" {
		t.Skip("set OPENCODE_URL to run against a live opencode serve")
	}
	r := &OpenCodeRunner{BaseURL: base, PollInterval: 300 * time.Millisecond}

	version, err := r.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	t.Logf("live opencode version %s", version)

	model := os.Getenv("OPENCODE_MODEL")
	if model == "" {
		t.Skip("set OPENCODE_MODEL to run a real prompt (costs money)")
	}

	dir := t.TempDir()
	log := filepath.Join(dir, "worker.log")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	res, err := r.Run(ctx, RunRequest{
		Prompt:  "Reply with exactly the word OK. Do not use any tools.",
		Cwd:     dir,
		Model:   model,
		LogPath: log,
		Timeout: 3 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Logf("session=%s exit=%d aborted=%v cost=$%.4f tokens=%d duration=%s",
		res.SessionID, res.Exit, res.Aborted, res.CostUSD, res.Tokens, res.Duration.Round(time.Millisecond))

	if res.SessionID == "" {
		t.Error("expected a session id back")
	}
	if res.Aborted {
		t.Error("attempt timed out against the live server")
	}
	if res.Tokens == 0 {
		t.Error("expected non-zero tokens — cost accounting feeds the budget guard")
	}
	if fi, err := os.Stat(log); err != nil || fi.Size() == 0 {
		t.Errorf("expected a non-empty transcript on disk: %v", err)
	}
}
