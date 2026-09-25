package orchestrator

import (
	"strings"
	"testing"
)

// A stale Codex sign-in fails as a 401 that names an API key; the message
// must lead with the fix instead.
func TestCodexAuthHint(t *testing.T) {
	got := codexAuthHint(`codex exited 1: {"type":"error","message":"unexpected status 401 Unauthorized: Incorrect API key provided"}`)
	if !strings.HasPrefix(got, "Codex's sign-in on this Mac was refused (401)") || !strings.Contains(got, "codex logout && codex login") {
		t.Errorf("no fix in front: %q", got)
	}
	if other := "codex exited 1: model not found"; codexAuthHint(other) != other {
		t.Errorf("an unrelated failure was changed: %q", codexAuthHint(other))
	}
}
