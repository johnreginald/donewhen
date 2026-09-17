package orchestrator

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// repoEnv reads a repository's own .env and returns it as KEY=VALUE lines,
// ready to append to os.Environ() — later entries win there, so a repository's
// value beats whatever the daemon inherited.
//
// WHY this exists at all: a check command inherits the daemon's environment,
// and the daemon has exactly one of those while ORCHESTRATOR_REPOS routes
// seven repositories. One global TEST_DATABASE_URL meant api-server's database
// tests ran against acme's schema and failed with `relation "service" does not
// exist` — an agent that had written perfect code would have been told it had
// failed. The repository is the authority on where its own test database
// lives, so the repository's file wins.
//
// It is read from the SOURCE repository, not the worktree: .env is gitignored
// everywhere it matters, and a disposable worktree is a clean checkout that
// never contains one.
//
// A missing file is the normal case and returns nothing, which leaves the
// command's environment exactly as it was. That matters — acme-console and
// acme-api have no .env and depend on the global value.
func repoEnv(repoDir string) []string {
	if repoDir == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(repoDir, ".env"))
	if err != nil {
		// Missing, unreadable, a directory — all the same answer. A check must
		// not fail because of how a repository is or is not configured; it
		// falls back to the inherited environment and runs.
		return nil
	}
	defer f.Close()

	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		if k, v, ok := parseEnvLine(s.Text()); ok {
			out = append(out, k+"="+v)
		}
	}
	if s.Err() != nil {
		// A partial read is still better than none: the lines already parsed
		// are valid, and refusing them would put us back on acme's database.
		return out
	}
	return out
}

// parseEnvLine splits one .env line. It handles what these files actually
// hold rather than a formal grammar: comments, blanks, an `export` prefix,
// and quoted values.
func parseEnvLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")

	// SplitN with n=2, not Split: a DSN carries `?sslmode=disable`, and
	// splitting on every = turns the one line this fix exists for into
	// nonsense.
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key = strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false
	}
	value = strings.TrimSpace(parts[1])
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}
