package orchestrator

import (
	"strings"
	"testing"
)

// A stale Codex sign-in fails as a 401 that names an API key; the message
// must lead with the fix instead.
func TestCodexAuthHint(t *testing.T) {
	got := codexAuthHint(`codex exited 1: {"type":"error","message":"unexpected status 401 Unauthorized: Incorrect API key provided"}`)
	if !strings.HasPrefix(got, "OpenAI refused Codex's sign-in") || !strings.Contains(got, "codex logout && codex login") {
		t.Errorf("no fix in front: %q", got)
	}
	if strings.Contains(got, "{") || strings.Contains(got, "Incorrect API key") {
		t.Errorf("the raw error is still in the message: %q", got)
	}
	if other := "codex exited 1: model not found"; codexAuthHint(other) != other {
		t.Errorf("an unrelated failure was changed: %q", codexAuthHint(other))
	}
}
