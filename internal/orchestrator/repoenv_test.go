package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"raenil/internal/models"
)

// The bug this file exists for: `orchestrator daemon` routes seven
// repositories and inherits one environment, so api-server's database tests
// ran against acme's schema and reported `relation "service" does not exist`.
// The suite was green. The gate said it had failed.

func TestARepositorysOwnEnvBeatsTheOneTheDaemonInherited(t *testing.T) {
	repo := t.TempDir()
	writeEnv(t, repo, "TEST_DATABASE_URL=postgres://api:api@localhost:5432/lsm_test?sslmode=disable\n")

	// Stand in for the daemon's global value — the wrong database.
	t.Setenv("TEST_DATABASE_URL", "postgres://postgres:test@localhost:55433/acme_test?sslmode=disable")

	got := runEcho(t, repo, "TEST_DATABASE_URL")

	want := "postgres://api:api@localhost:5432/lsm_test?sslmode=disable"
	if got != want {
		t.Fatalf("check saw %q, want the repository's own %q", got, want)
	}
}

// The query string is the whole point: splitting a DSN on every `=` turns
// `?sslmode=disable` into a different value, and the check would then fail
// for a reason nobody would look for.
func TestADSNSurvivesTheEqualsSignInItsQueryString(t *testing.T) {
	repo := t.TempDir()
	writeEnv(t, repo, "DSN=postgres://u:p@localhost:5432/db?sslmode=disable&timezone=UTC\n")

	got := runEcho(t, repo, "DSN")

	want := "postgres://u:p@localhost:5432/db?sslmode=disable&timezone=UTC"
	if got != want {
		t.Fatalf("DSN came back as %q, want %q", got, want)
	}
}

// acme-console and acme-api ship no .env and depend on the daemon's value.
// Loading must be an addition, never a replacement.
func TestARepositoryWithNoEnvFileLeavesTheInheritedEnvironmentAlone(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("TEST_DATABASE_URL", "postgres://postgres:test@localhost:55433/acme_test?sslmode=disable")

	got := runEcho(t, repo, "TEST_DATABASE_URL")

	want := "postgres://postgres:test@localhost:55433/acme_test?sslmode=disable"
	if got != want {
		t.Fatalf("check saw %q, want the inherited %q", got, want)
	}
}

// A repository's .env holds whatever a person typed in it. None of these
// shapes may become a variable, and none may stop the ones after it being
// read — a check that dies on a comment is worse than one with no .env.
func TestCommentsBlanksAndExportPrefixesAreHandledTheWayAFileActuallyLooks(t *testing.T) {
	repo := t.TempDir()
	writeEnv(t, repo, `# a comment

export EXPORTED=yes
QUOTED="double"
SINGLE='single'
  SPACED  =  padded

NOT_A_PAIR
LAST=reached
`)

	env := repoEnv(repo)
	got := map[string]string{}
	for _, kv := range env {
		k, v, _ := parseEnvLine(kv)
		got[k] = v
	}

	for _, c := range []struct{ key, want string }{
		{"EXPORTED", "yes"},
		{"QUOTED", "double"},
		{"SINGLE", "single"},
		{"SPACED", "padded"},
		{"LAST", "reached"},
	} {
		if got[c.key] != c.want {
			t.Errorf("%s = %q, want %q", c.key, got[c.key], c.want)
		}
	}
	if _, ok := got["NOT_A_PAIR"]; ok {
		t.Errorf("a line with no = became a variable")
	}
	if len(got) != 5 {
		t.Errorf("read %d variables, want 5: %v", len(got), got)
	}
}

// A check must run whatever state the repository is in. An .env that cannot
// be read is a configuration problem, not a reason to report a verdict.
func TestAnUnreadableEnvDoesNotStopTheCheckRunning(t *testing.T) {
	repo := t.TempDir()
	// A directory where the file should be: os.Open succeeds on some systems
	// and the read then fails, which is the awkward case.
	if err := os.Mkdir(filepath.Join(repo, ".env"), 0o755); err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}
	t.Setenv("TEST_DATABASE_URL", "inherited")

	got := runEcho(t, repo, "TEST_DATABASE_URL")

	if got != "inherited" {
		t.Fatalf("check saw %q; an unreadable .env must fall back to the inherited environment", got)
	}
}

// ── Fixture ─────────────────────────────────────────────────────────────

func writeEnv(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

// runEcho drives a real deterministic check — the whole path, not repoEnv in
// isolation — and returns what the command saw for one variable.
func runEcho(t *testing.T, repoDir, key string) string {
	t.Helper()

	work := t.TempDir()
	runRoot := t.TempDir()
	dir, err := NewRunDir(runRoot, "TST-1", 1)
	if err != nil {
		t.Fatalf("NewRunDir: %v", err)
	}
	out := filepath.Join(work, "seen.txt")

	e := &Evaluator{WorkDir: work, RepoDir: repoDir, Dir: dir}
	spec, err := json.Marshal(map[string]string{
		"cmd": "printf '%s' \"$" + key + "\" > " + out,
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	cs := parse(t, []models.Criterion{{
		Body:      "the check reports what it saw for " + key,
		Kind:      models.CriterionDeterministic,
		CheckSpec: spec,
	}})
	ev, err := e.Evaluate(context.Background(), cs, Diff{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !ev[0].Pass {
		t.Fatalf("the echo check itself failed: detail=%q err=%q", ev[0].Detail, ev[0].Err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read what the check saw: %v", err)
	}
	return string(b)
}
