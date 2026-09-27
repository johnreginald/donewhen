package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"raenil/internal/models"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// After a merge on the remote, the local base catches up, whether or not it
// is the branch checked out.
func TestUpdateBaseFastForwards(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	gitT(t, root, "init", "-q", "--bare", "-b", "main", remote)
	seed := filepath.Join(root, "seed")
	gitT(t, root, "clone", "-q", remote, seed)
	os.WriteFile(filepath.Join(seed, "a.txt"), []byte("1"), 0o644)
	gitT(t, seed, "add", ".")
	gitT(t, seed, "commit", "-qm", "one")
	gitT(t, seed, "push", "-q", "origin", "HEAD:main")

	local := filepath.Join(root, "local")
	gitT(t, root, "clone", "-q", remote, local)

	os.WriteFile(filepath.Join(seed, "a.txt"), []byte("2"), 0o644)
	gitT(t, seed, "commit", "-qam", "merged pr")
	gitT(t, seed, "push", "-q", "origin", "HEAD:main")
	want := gitT(t, seed, "rev-parse", "HEAD")

	o := &Orchestrator{}
	if got := defaultBranch(context.Background(), local); got != "main" {
		t.Fatalf("default branch = %q", got)
	}
	if err := o.updateBase(context.Background(), local); err != nil {
		t.Fatal(err)
	}
	if got := gitT(t, local, "rev-parse", "main"); got != want {
		t.Errorf("checked-out main = %s, want %s", got, want)
	}

	// Not checked out: the ref itself moves.
	gitT(t, local, "checkout", "-qb", "other")
	os.WriteFile(filepath.Join(seed, "a.txt"), []byte("3"), 0o644)
	gitT(t, seed, "commit", "-qam", "another")
	gitT(t, seed, "push", "-q", "origin", "HEAD:main")
	want = gitT(t, seed, "rev-parse", "HEAD")
	if err := o.updateBase(context.Background(), local); err != nil {
		t.Fatal(err)
	}
	if got := gitT(t, local, "rev-parse", "main"); got != want {
		t.Errorf("main ref = %s, want %s", got, want)
	}
}

func TestOpenPRWithoutRemoteKeepsTheBranch(t *testing.T) {
	repo := t.TempDir()
	gitT(t, repo, "init", "-q")
	o := &Orchestrator{Cfg: Config{Repo: repo}}
	url, err := o.OpenPR(context.Background(), models.Issue{Key: "T-1"}, repo, "ticket/t-1-attempt-1", "")
	if err != nil || url != "" {
		t.Errorf("OpenPR with no remote = %q, %v; want nothing and no error", url, err)
	}
}
