package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"raenil/internal/models"
)

// Passed work leaves the machine as a pull request, and the user's approval
// merges it: trunk-based flow. A ticket's branch lives only until it lands on
// the base branch; dependents then start from the base, not from a stack of
// unmerged branches.

// defaultBranch is the remote's default branch, e.g. "main".
func defaultBranch(ctx context.Context, repo string) string {
	if out, err := git(ctx, repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if b := strings.TrimPrefix(strings.TrimSpace(out), "origin/"); b != "" {
			return b
		}
	}
	return "main"
}

// hasOrigin reports whether the repository has a remote to push to.
func hasOrigin(ctx context.Context, repo string) bool {
	_, err := git(ctx, repo, "remote", "get-url", "origin")
	return err == nil
}

// gh runs the GitHub CLI in dir.
func gh(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("gh %s: %v: %s", strings.Join(args[:min(2, len(args))], " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// OpenPR pushes a ticket's branch and opens its pull request, or finds the
// one already open. A repository with no remote has nowhere to open one:
// that is not an error, the work stays on the branch.
func (o *Orchestrator) OpenPR(ctx context.Context, issue models.Issue, wtPath, branch, body string) (string, error) {
	repo := o.Cfg.withDefaults().Repo
	if !hasOrigin(ctx, repo) {
		o.logf("no origin remote: %s stays on its branch", branch)
		return "", nil
	}
	if out, err := git(ctx, wtPath, "push", "--force-with-lease", "-u", "origin", branch); err != nil {
		return "", fmt.Errorf("push %s: %v: %s", branch, err, strings.TrimSpace(out))
	}
	if out, err := gh(ctx, repo, "pr", "view", branch, "--json", "url,state", "-q", `select(.state=="OPEN") | .url`); err == nil {
		if url := strings.TrimSpace(out); url != "" {
			o.logf("pull request already open: %s", url)
			return url, nil
		}
	}
	title := issue.Key + ": " + issue.Title
	out, err := gh(ctx, repo, "pr", "create", "--head", branch, "--base", defaultBranch(ctx, repo),
		"--title", title, "--body", body)
	if err != nil {
		return "", err
	}
	url := lastLine(out)
	o.logf("pull request %s", url)
	return url, nil
}

// prBody is the pull request's description: the ticket and the checks that
// passed. The review itself lives on the ticket page.
func (o *Orchestrator) prBody(issue models.Issue, v Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Raenil ticket **%s** — %s\n\n", issue.Key, issue.Title)
	if o.Raenil != nil && o.Raenil.BaseURL != "" {
		fmt.Fprintf(&b, "%s/issue/%s\n\n", strings.TrimRight(o.Raenil.BaseURL, "/"), issue.Key)
	}
	if len(v.Passed) > 0 || len(v.Failed) > 0 {
		fmt.Fprintf(&b, "Checks: %d passed, %d failed.\n", len(v.Passed), len(v.Failed))
	}
	b.WriteString("\nReview and merge from the ticket page.\n")
	return b.String()
}

// Merge lands a ticket's pull request on the base branch — squashed — and
// moves the ticket to Done. The user's approval is what calls it; nothing
// merges on its own.
func (o *Orchestrator) Merge(ctx context.Context, ref string) (string, error) {
	scoped, issue, err := o.forTicket(ctx, ref)
	if err != nil {
		return "", err
	}
	o = scoped
	cfg := o.Cfg.withDefaults()
	repo := cfg.Repo

	pr := ""
	if issue.PRURL != nil {
		pr = strings.TrimSpace(*issue.PRURL)
	}
	branch := ""
	if issue.GitBranch != nil {
		branch = strings.TrimSpace(*issue.GitBranch)
	}
	if pr == "" && branch != "" {
		if out, err := gh(ctx, repo, "pr", "view", branch, "--json", "url", "-q", ".url"); err == nil {
			pr = strings.TrimSpace(out)
		}
	}
	if pr == "" {
		return "", errors.New(issue.Key + " has no pull request to merge — finish it first")
	}

	// A worktree still holding the branch stops gh deleting it.
	if wt, err := o.FindWorktree(issue.Key); err == nil {
		w := &Worktree{Repo: repo, Path: wt, Branch: branchForWorktree(wt)}
		_ = w.Detach(context.WithoutCancel(ctx))
	}
	if _, err := gh(ctx, repo, "pr", "merge", pr, "--squash", "--delete-branch"); err != nil {
		return "", err
	}
	sha := ""
	if out, err := gh(ctx, repo, "pr", "view", pr, "--json", "mergeCommit", "-q", ".mergeCommit.oid"); err == nil {
		sha = strings.TrimSpace(out)
	}
	o.logf("merged %s (%s)", pr, sha[:min(8, len(sha))])

	// The base must hold the merge before the next ticket cuts its worktree.
	if err := o.updateBase(ctx, repo); err != nil {
		o.logf("warning: merged, but the local base is not updated: %v", err)
	}
	// Its branch is gone from the remote; drop the local one too, so no later
	// ticket merges the unsquashed commits on top of the squash.
	if branch != "" {
		_, _ = git(context.WithoutCancel(ctx), repo, "branch", "-D", branch)
	}
	if sha != "" {
		_ = o.Raenil.LinkCommit(ctx, issue.ID, sha, issue.Title+" (merged)")
	}
	if err := o.Raenil.SetState(ctx, issue.ID, "Done"); err != nil {
		return pr, fmt.Errorf("merged, but could not move %s to Done: %w", issue.Key, err)
	}
	o.logf("%s → Done", issue.Key)
	return pr, nil
}

// updateBase brings the local base branch up to the remote's, without
// touching a checkout that has work in it.
func (o *Orchestrator) updateBase(ctx context.Context, repo string) error {
	base := defaultBranch(ctx, repo)
	if _, err := git(ctx, repo, "fetch", "--quiet", "origin", base); err != nil {
		return err
	}
	cur, _ := git(ctx, repo, "rev-parse", "--abbrev-ref", "HEAD")
	if strings.TrimSpace(cur) != base {
		// Not checked out here: move the ref itself (fast-forward only).
		_, err := git(ctx, repo, "fetch", "--quiet", "origin", base+":"+base)
		return err
	}
	if status, _ := git(ctx, repo, "status", "--porcelain", "--untracked-files=no"); strings.TrimSpace(status) != "" {
		return errors.New("the base is checked out with uncommitted changes; pull it yourself")
	}
	_, err := git(ctx, repo, "merge", "--ff-only", "--quiet", "origin/"+base)
	return err
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
