package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newRepo builds a throwaway git repo with one source file and one test file.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "Test")
	write(t, filepath.Join(dir, "src"), "add.ts", "export const add = (a:number,b:number) => a - b\n")
	write(t, filepath.Join(dir, "src"), "add.test.ts", "expect(add(2,3)).toBe(5)\n")
	run("add", "-A")
	run("commit", "-qm", "init")
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreeLifecycleAndDiff(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	wtPath := filepath.Join(t.TempDir(), "wt")

	wt, err := AddWorktree(ctx, repo, wtPath, "ticket/TST-1", "HEAD")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	// A worker fixes the bug, adds an untracked file, and removes a doc.
	write(t, filepath.Join(wtPath, "src"), "add.ts", "export const add = (a:number,b:number) => a + b\n")
	write(t, filepath.Join(wtPath, "src"), "helper.ts", "export const noop = () => {}\n")

	d, err := StageAndDiff(ctx, wtPath, "HEAD")
	if err != nil {
		t.Fatalf("StageAndDiff: %v", err)
	}

	got := map[string]DiffFile{}
	for _, f := range d.Files {
		got[f.Path] = f
	}
	if _, ok := got["src/add.ts"]; !ok {
		t.Errorf("modified file missing from diff; got %v", got)
	}
	// The untracked file is the case a naive `git diff` would miss entirely.
	if f, ok := got["src/helper.ts"]; !ok {
		t.Error("untracked new file must appear in the diff")
	} else if f.Status != "added" {
		t.Errorf("helper.ts status = %q, want added", f.Status)
	}
	if f := got["src/add.ts"]; f.Patch == "" {
		t.Error("expected per-file patch text for content-aware gates")
	}

	if err := wt.Remove(ctx); err != nil {
		t.Errorf("Remove: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory should be gone after Remove")
	}
}

// End to end: a worker that deletes the failing test instead of fixing the code
// must be blocked by the gate, using a real git diff rather than a synthetic one.
func TestDeletedTestIsBlockedEndToEnd(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	wtPath := filepath.Join(t.TempDir(), "wt")

	wt, err := AddWorktree(ctx, repo, wtPath, "ticket/TST-2", "HEAD")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	defer wt.Remove(ctx)

	if err := os.Remove(filepath.Join(wtPath, "src", "add.test.ts")); err != nil {
		t.Fatal(err)
	}

	d, err := StageAndDiff(ctx, wtPath, "HEAD")
	if err != nil {
		t.Fatalf("StageAndDiff: %v", err)
	}
	res, err := EvalPolicy(PolicyCheck{Policy: "tests_not_weakened"}, d)
	if err != nil {
		t.Fatalf("EvalPolicy: %v", err)
	}
	if res.Pass {
		t.Fatalf("deleting the test must fail the gate; diff was %+v", d.Files)
	}
	t.Logf("gate correctly refused: %s", res.Detail)
}

// A ticket built on a blocker still in review starts with the blocker's code
// in place, and its diff holds only its own change.
func TestMergeBlockerBranches(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	// The blocker's work, on its own branch, not merged.
	blocker, err := AddWorktree(ctx, repo, filepath.Join(t.TempDir(), "blk"), "ticket/blk-1", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(blocker.Path, "api"), "go.mod", "module x\n")
	if _, err := Commit(ctx, blocker.Path, "blocker work"); err != nil {
		t.Fatal(err)
	}

	dep, err := AddWorktree(ctx, repo, filepath.Join(t.TempDir(), "dep"), "ticket/dep-1", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if WorktreeBase(ctx, dep.Path) != "" {
		t.Fatal("a fresh worktree already has a base")
	}
	if err := MergeBlockerBranches(ctx, dep.Path, []string{"ticket/blk-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dep.Path, "api", "go.mod")); err != nil {
		t.Fatalf("the blocker's work is not in place: %v", err)
	}
	base := WorktreeBase(ctx, dep.Path)
	if base == "" {
		t.Fatal("no base recorded after building on a blocker")
	}

	write(t, filepath.Join(dep.Path, "api"), "main.go", "package main\n")
	d, err := StageAndDiff(ctx, dep.Path, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Files) != 1 || d.Files[0].Path != "api/main.go" {
		t.Errorf("diff = %v, want only api/main.go", d.Files)
	}

	// Two blockers whose work collides: the ticket does not start.
	other, _ := AddWorktree(ctx, repo, filepath.Join(t.TempDir(), "oth"), "ticket/oth-1", "HEAD")
	write(t, filepath.Join(other.Path, "api"), "go.mod", "module y\n")
	Commit(ctx, other.Path, "other work")
	clash, _ := AddWorktree(ctx, repo, filepath.Join(t.TempDir(), "clash"), "ticket/clash-1", "HEAD")
	if err := MergeBlockerBranches(ctx, clash.Path, []string{"ticket/blk-1", "ticket/oth-1"}); err == nil {
		t.Error("conflicting blockers were combined")
	}
}

// An agent that switches the worktree to another branch is caught before its
// work is committed there.
func TestOnTicketBranch(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	wt, err := AddWorktree(ctx, repo, filepath.Join(t.TempDir(), "w"), "ticket/t-1", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := onTicketBranch(ctx, wt.Path, "ticket/t-1"); err != nil {
		t.Errorf("on its own branch: %v", err)
	}
	if _, err := git(ctx, wt.Path, "checkout", "-q", "-b", "elsewhere"); err != nil {
		t.Fatal(err)
	}
	if err := onTicketBranch(ctx, wt.Path, "ticket/t-1"); err == nil {
		t.Error("a worktree on another branch passed")
	}
}
