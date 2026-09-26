package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"raenil/internal/models"
)

// FindWorktree locates the kept worktree for a ticket, newest attempt first.
func (o *Orchestrator) FindWorktree(ticketKey string) (string, error) {
	root := filepath.Join(o.Cfg.withDefaults().RunRoot, "worktrees")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("no worktrees under %s: %w", root, err)
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), ticketKey+"-") {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no worktree for %s — run `orchestrator work %s --handoff` first", ticketKey, ticketKey)
	}
	// Newest attempt by number: as strings, attempt 10 would sort before 9.
	n := func(name string) int {
		v, _ := strconv.Atoi(strings.TrimPrefix(name, ticketKey+"-"))
		return v
	}
	sort.Slice(matches, func(i, j int) bool { return n(matches[i]) > n(matches[j]) })
	return filepath.Join(root, matches[0]), nil
}

// Verify re-runs a ticket's criteria against its existing worktree, with no
// agent involved.
//
// This is how a reviewer checks their own fix: the same engine, the same
// criteria, on whatever the tree now contains. Who wrote the code makes no
// difference to what decides whether it is acceptable.
func (o *Orchestrator) Verify(ctx context.Context, ref string, attempt int) (Verdict, []Evidence, error) {
	var v Verdict

	scoped, issue, err := o.forTicket(ctx, ref)
	if err != nil {
		return v, nil, err
	}
	o = scoped
	cfg := o.Cfg.withDefaults()

	stored, err := o.Raenil.Criteria(ctx, issue.ID)
	if err != nil {
		return v, nil, err
	}
	criteria, err := ParseCriteria(stored)
	if err != nil {
		return v, nil, fmt.Errorf("issue %s has an unusable checklist: %w", issue.Key, err)
	}

	wtPath, err := o.FindWorktree(issue.Key)
	if err != nil {
		return v, nil, err
	}
	o.logf("verifying %s in %s", issue.Key, wtPath)

	diff, err := StageAndDiff(ctx, wtPath, cfg.BaseRef)
	if err != nil {
		return v, nil, fmt.Errorf("read diff: %w", err)
	}

	runDir, err := NewRunDir(cfg.RunRoot, issue.Key, attempt)
	if err != nil {
		return v, nil, err
	}
	byIndex := map[int]models.Criterion{}
	for i, c := range stored {
		byIndex[i] = c
	}
	ev := &Evaluator{
		WorkDir: wtPath,
		RepoDir: cfg.Repo,
		Dir:     runDir,
		Judge:   o.Judge,
		OnEvidence: func(e Evidence) {
			c, ok := byIndex[e.CriterionIndex]
			if !ok || c.ID == "" {
				return
			}
			ref := fmt.Sprintf("%s/evidence.json#%d", runDir.Path(), e.CriterionIndex)
			// Tick what passed; un-tick what no longer does, so a fix that broke
			// something else cannot leave a stale green on the record.
			if err := o.Raenil.TickCriterion(ctx, c.ID, e.Pass, ref); err != nil {
				o.logf("warning: could not update criterion %d: %v", e.CriterionIndex+1, err)
			}
		},
	}
	evidence, err := ev.Evaluate(ctx, criteria, diff)
	if err != nil {
		return v, nil, err
	}
	for _, e := range evidence {
		o.logf("  [%s] %s: pass=%v %s%s", e.Kind, e.CriterionText, e.Pass, e.Detail, e.Err)
	}

	v = Summarise(issue.Key, attempt, criteria, evidence, diff)
	v.Runner, v.Model = "verify", ""
	if err := runDir.WriteVerdict(v); err != nil {
		return v, evidence, err
	}
	o.logf("verdict: %s", v.Status)
	return v, evidence, nil
}

// Finish commits a reviewer's changes as their own commit, records the
// artifact, and moves the ticket to review.
//
// It re-verifies first and refuses when a gating criterion fails. A reviewer may
// improve the code; deciding that it is acceptable stays with the checks, or the
// senior becomes another model marking its own work.
func (o *Orchestrator) Finish(ctx context.Context, ref string, attempt int) (Verdict, error) {

	scoped, _, err := o.forTicket(ctx, ref)
	if err != nil {
		return Verdict{}, err
	}
	o = scoped
	cfg := o.Cfg.withDefaults()

	v, _, err := o.Verify(ctx, ref, attempt)
	if err != nil {
		return v, err
	}
	if v.Status != StatusPassed {
		return v, fmt.Errorf("%s still fails %d gating criteria — fix them, move the ticket to Blocked, "+
			"or change the criteria if they are wrong", ref, len(v.Failed))
	}

	issue, err := o.Raenil.Issue(ctx, ref)
	if err != nil {
		return v, err
	}
	wtPath, err := o.FindWorktree(issue.Key)
	if err != nil {
		return v, err
	}

	sha, cerr := Commit(ctx, wtPath, fmt.Sprintf("%s: review fixes", issue.Key))
	switch {
	case cerr != nil:
		// Nothing to commit means the reviewer changed nothing, which is a fine
		// outcome: the worker's commit already stands.
		o.logf("reviewer made no changes; the worker's commit stands")
	default:
		_ = o.Raenil.LinkCommit(ctx, issue.ID, sha, issue.Title+" (review fixes)")
		o.logf("review commit %s", sha[:min(8, len(sha))])
	}

	if err := o.Raenil.SetState(ctx, issue.ID, cfg.StateInReview); err != nil {
		return v, fmt.Errorf("could not move to %s: %w", cfg.StateInReview, err)
	}

	// Release the checkout but keep the branch. A worktree left behind holds the
	// branch checked out, and git refuses to merge a branch that is checked out
	// somewhere else — so the reviewer's own worktree would block the merge.
	wt := &Worktree{Repo: cfg.Repo, Path: wtPath, Branch: branchForWorktree(wtPath)}
	if err := wt.Detach(context.WithoutCancel(ctx)); err != nil {
		o.logf("warning: could not release the worktree at %s: %v", wtPath, err)
	} else {
		o.logf("released the worktree; branch kept for merging")
	}

	o.logf("%s → %s", issue.Key, cfg.StateInReview)
	return v, nil
}

// ReviewDiff is the ticket's kept worktree against where it branched from, as
// Verify leaves it staged — what a reviewer is looking at.
func (o *Orchestrator) ReviewDiff(ctx context.Context, ref string) (string, error) {
	scoped, issue, err := o.forTicket(ctx, ref)
	if err != nil {
		return "", err
	}
	wt, err := scoped.FindWorktree(issue.Key)
	if err != nil {
		return "", err
	}
	// The worktree's HEAD is already the worker's commit, so the diff has to
	// run from where its branch left the main checkout — the whole change,
	// the worker's commit and any fixes since.
	base := scoped.Cfg.withDefaults().BaseRef
	if main, err := git(ctx, scoped.Cfg.Repo, "rev-parse", "HEAD"); err == nil {
		if mb, err := git(ctx, wt, "merge-base", "HEAD", strings.TrimSpace(main)); err == nil {
			base = strings.TrimSpace(mb)
		}
	}
	// Built on other tickets' branches: those are theirs to review, so the
	// diff starts after them.
	if b := WorktreeBase(ctx, wt); b != "" {
		base = b
	}
	return gitRaw(ctx, wt, "diff", "--cached", base)
}

// branchForWorktree reads the branch a worktree has checked out.
func branchForWorktree(path string) string {
	out, err := git(context.Background(), path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
