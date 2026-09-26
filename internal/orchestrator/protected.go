package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Places on the runner Mac a worker has no business reading: the user's
// iCloud-synced folders — reading them makes macOS stop the run with a
// permission prompt for the user — and the runner's own settings and logins.
// A worker needs its worktree and its repository, nothing here.
var protectedHomeDirs = []string{
	"Desktop",
	"Documents",
	"Library/Mobile Documents",
	".config/raenil",
	".raenil/connections",
	".ssh",
}

// protectedPaths are protectedHomeDirs as absolute paths.
func protectedPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	out := make([]string, len(protectedHomeDirs))
	for i, d := range protectedHomeDirs {
		out[i] = filepath.Join(home, d)
	}
	return out
}

// claudeProtectedDeny are Claude Code deny rules for the protected paths.
// Measured (claude 2.1.283): a Read deny rule refuses the Read tool and also
// shell commands that read there, such as cat and grep -r.
func claudeProtectedDeny() []string {
	var rules []string
	for _, p := range protectedPaths() {
		// "//" marks an absolute path in a permission rule.
		rules = append(rules, "Read(/"+p+"/**)", "Edit(/"+p+"/**)")
	}
	return rules
}

// claudeProtectionSettings is the --settings JSON carrying those rules.
func claudeProtectionSettings() string {
	b, _ := json.Marshal(map[string]any{"permissions": map[string]any{"deny": claudeProtectedDeny()}})
	return string(b)
}

// touchesProtected reports whether a refused call named a protected path.
// Such a refusal is the protection working, not a command the user should
// allow: it must not stop the ticket the way a missing allowance does.
func touchesProtected(input string) bool {
	home, _ := os.UserHomeDir()
	for _, d := range protectedHomeDirs {
		if (home != "" && strings.Contains(input, filepath.Join(home, d))) ||
			strings.Contains(input, "~/"+d) || strings.Contains(input, "$HOME/"+d) {
			return true
		}
	}
	return false
}

// protectedPrompt tells a worker where it may and may not look.
const protectedPrompt = "\n\n## Where you may look\n\nWork only inside this worktree and the repository it belongs to. " +
	"Do not read, list or search your home folder, ~/Desktop, ~/Documents, iCloud Drive, other projects, or the " +
	"runner's own settings (~/.config/raenil, ~/.raenil). Everything the ticket needs is in the repository; if " +
	"something is missing, say so instead of looking elsewhere.\n"

// openCodePermissions are the session rules a work run is created with: the
// repository may be read without asking, and the protected paths may not be
// read at all, by the file tools or by a shell command naming them. Later
// rules win. Measured (opencode 1.18.32): a session created with these reads
// the repository with no permission prompt and refuses the rest, cat
// included.
func openCodePermissions(repo string) []map[string]string {
	var rules []map[string]string
	if repo != "" {
		rules = append(rules, map[string]string{"permission": "external_directory", "pattern": filepath.Clean(repo) + "*", "action": "allow"})
	}
	for _, p := range protectedPaths() {
		for _, perm := range []string{"external_directory", "read", "edit", "list", "glob", "grep"} {
			rules = append(rules, map[string]string{"permission": perm, "pattern": p + "*", "action": "deny"})
		}
		rules = append(rules, map[string]string{"permission": "bash", "pattern": "*" + p + "*", "action": "deny"})
	}
	// A shell command can name them from home, too.
	for _, d := range protectedHomeDirs {
		for _, prefix := range []string{"~/", "$HOME/"} {
			rules = append(rules, map[string]string{"permission": "bash", "pattern": "*" + prefix + d + "*", "action": "deny"})
		}
	}
	return rules
}
