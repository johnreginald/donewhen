package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Worktree is a disposable checkout a worker is allowed to write to freely.
// The orchestrator is the only thing that decides what leaves it.
type Worktree struct {
	// Repo is the source repository.
	Repo string
	// Path is the worktree directory.
	Path string
	// Branch is the branch created for this attempt.
	Branch string
}

// git runs a git command and returns its stdout.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// AddWorktree creates a worktree for a ticket at path, on a new branch off base.
// Workers never touch the main checkout.
func AddWorktree(ctx context.Context, repo, path, branch, base string) (*Worktree, error) {
	if base == "" {
		base = "HEAD"
	}
	if _, err := git(ctx, repo, "worktree", "add", "-b", branch, path, base); err != nil {
		return nil, err
	}
	return &Worktree{Repo: repo, Path: path, Branch: branch}, nil
}

// Remove deletes the worktree and its branch. Safe to call on a partially
// created worktree — a daemon reclaiming a dead lease must be able to clean up
// whatever state it finds.
func (w *Worktree) Remove(ctx context.Context) error { return w.remove(ctx, true) }

// Detach removes the worktree but keeps the branch, for an attempt whose work
// is worth keeping and reviewing.
func (w *Worktree) Detach(ctx context.Context) error { return w.remove(ctx, false) }

func (w *Worktree) remove(ctx context.Context, dropBranch bool) error {
	var errs []string
	if _, err := git(ctx, w.Repo, "worktree", "remove", "--force", w.Path); err != nil {
		// The directory may already be gone; prune catches that case.
		if _, perr := git(ctx, w.Repo, "worktree", "prune"); perr != nil {
			errs = append(errs, err.Error())
		}
		_ = os.RemoveAll(w.Path)
	}
	if dropBranch && w.Branch != "" {
		if _, err := git(ctx, w.Repo, "branch", "-D", w.Branch); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("remove worktree: %s", strings.Join(errs, "; "))
	}
	return nil
}

// StageAndDiff stages everything a worker produced — including files it never
// added — and returns the resulting diff against base. Untracked files are the
// common case for a worker adding a module, and a gate that cannot see them is
// not a gate.
func StageAndDiff(ctx context.Context, dir, base string) (Diff, error) {
	if base == "" {
		base = "HEAD"
	}
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return Diff{}, err
	}
	return diffCached(ctx, dir, base)
}

func diffCached(ctx context.Context, dir, base string) (Diff, error) {
	numstat, err := git(ctx, dir, "diff", "--cached", "--numstat", base)
	if err != nil {
		return Diff{}, err
	}
	nameStatus, err := git(ctx, dir, "diff", "--cached", "--name-status", base)
	if err != nil {
		return Diff{}, err
	}
	patch, err := git(ctx, dir, "diff", "--cached", base)
	if err != nil {
		return Diff{}, err
	}

	statusOf := parseNameStatus(nameStatus)
	patchOf := splitPatchByFile(patch)

	var d Diff
	for _, line := range strings.Split(strings.TrimSpace(numstat), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		// Binary files report "-" for both counts.
		ins, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		p := parts[2]
		d.Files = append(d.Files, DiffFile{
			Path:       p,
			Status:     statusOf[p],
			Insertions: ins,
			Deletions:  del,
			Patch:      patchOf[p],
		})
	}
	return d, nil
}

// parseNameStatus maps each path to added/modified/deleted/renamed.
func parseNameStatus(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		// A rename line is "R100\told\tnew"; the new path is what matters.
		p := fields[len(fields)-1]
		switch fields[0][0] {
		case 'A':
			m[p] = "added"
		case 'D':
			m[p] = "deleted"
		case 'R':
			m[p] = "renamed"
		default:
			m[p] = "modified"
		}
	}
	return m
}

// splitPatchByFile carves a combined patch into per-file hunks, so content-aware
// gates can reason about one file at a time.
func splitPatchByFile(patch string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(patch) == "" {
		return out
	}
	var cur string
	var buf strings.Builder
	flush := func() {
		if cur != "" {
			out[cur] = buf.String()
		}
		buf.Reset()
	}
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			cur = pathFromDiffHeader(line)
			continue
		}
		if cur != "" {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	flush()
	return out
}

// pathFromDiffHeader pulls the b/ path out of `diff --git a/x b/x`.
func pathFromDiffHeader(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return ""
	}
	return strings.TrimPrefix(fields[3], "b/")
}
