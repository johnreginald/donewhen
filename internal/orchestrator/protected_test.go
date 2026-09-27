package orchestrator

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestProtectedPaths(t *testing.T) {
	home, _ := os.UserHomeDir()
	rules := strings.Join(claudeProtectedDeny(), " ")
	for _, want := range []string{"Read(/" + home + "/Desktop/**)", "Read(/" + home + "/.config/raenil/**)"} {
		if !strings.Contains(rules, want) {
			t.Errorf("deny rules lack %s: %s", want, rules)
		}
	}
	r := &ClaudeRunner{}
	if w := r.args(RunRequest{}, "m"); !slices.Contains(w, "--settings") {
		t.Errorf("work args carry no protection settings: %v", w)
	}
	for in, want := range map[string]bool{
		"cat " + home + "/.config/raenil/orchestrator.env": true,
		"ls ~/Desktop":         true,
		"go test ./...":        false,
		home + "/Project/x.go": false,
	} {
		if got := touchesProtected(in); got != want {
			t.Errorf("touchesProtected(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestOpenCodePermissions(t *testing.T) {
	home, _ := os.UserHomeDir()
	rules := openCodePermissions("/src/repo", "/runs/worktrees/K-1-1")
	has := func(perm, pattern, action string) bool {
		for _, r := range rules {
			if r["permission"] == perm && r["pattern"] == pattern && r["action"] == action {
				return true
			}
		}
		return false
	}
	if !has("external_directory", "/runs/worktrees*", "allow") {
		t.Errorf("the worktrees folder is not readable without asking: %v", rules)
	}
	if !has("external_directory", "/src/repo*", "allow") {
		t.Errorf("the repository is not readable without asking: %v", rules)
	}
	if !has("read", home+"/Documents*", "deny") || !has("bash", "*~/.config/raenil*", "deny") {
		t.Errorf("protected paths are not denied: %v", rules)
	}
	// The allow comes first, so a later deny wins inside it.
	if rules[0]["action"] != "allow" {
		t.Errorf("allow must come before the denies: %v", rules[0])
	}
}
