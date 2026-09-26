package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"raenil/internal/models"
)

// Config is what one orchestrator needs to work a ticket.
type Config struct {
	// RunRoot holds run directories, e.g. ".orchestrator".
	RunRoot string
	// Repo is the git repository tickets are worked in.
	Repo string
	// BaseRef is what worktrees branch from.
	BaseRef string
	// Model is "provider/model" for the worker.
	Model string
	// Timeout bounds one attempt.
	Timeout time.Duration
	// StateInProgress and StateInReview name the workflow states to move through.
	StateInProgress string
	StateInReview   string
	// Handoff stops after the worker and the criteria have run: the work is
	// committed, the worktree is kept, and the tracker is left alone so a
	// reviewer decides what happens next.
	Handoff bool
}

func (c Config) withDefaults() Config {
	if c.RunRoot == "" {
		c.RunRoot = ".orchestrator"
	}
	// Absolute, against the repo, once — so runs, leases and worktrees all
	// name the same place whatever directory the process was started from.
	//
	// The default is relative, and a relative RunRoot means every consumer
	// resolves it against its own cwd. That is how `orchestrator status`
	// came to report "no tickets held" while three leases were live: it was
	// run from a different directory and read an empty leases folder. A
	// status command that answers confidently and wrongly is worse than one
	// that fails, because nobody re-checks it.
	if !filepath.IsAbs(c.RunRoot) && c.Repo != "" {
		if abs, err := filepath.Abs(filepath.Join(c.Repo, c.RunRoot)); err == nil {
			c.RunRoot = abs
		}
	}
	if c.BaseRef == "" {
		c.BaseRef = "HEAD"
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultRunTimeout
	}
	if c.StateInProgress == "" {
		c.StateInProgress = "In Progress"
	}
	if c.StateInReview == "" {
		c.StateInReview = "In Review"
	}
	return c
}

// Orchestrator works one ticket end to end. It is the control plane: it decides
// pass and fail from evidence, and the model it drives never gets a vote.
type Orchestrator struct {
	Raenil *RaenilClient
	Runner Runner
	Cfg    Config
	// Judge evaluates judgment criteria. Nil leaves them unevaluated, which is
	// safe: they are advisory and never gate a ticket either way.
	Judge JudgeFunc
	// Leases, when set, claims a ticket for the duration of Work so two runs
	// cannot take the same one. The daemon always sets it; a hand-run should too.
	Leases *LeaseManager
	// Runners is the pool a ticket may choose from with a runner: label. Empty
	// means every ticket uses Runner.
	Runners RunnerSet
	// Repos maps a repo label to a checkout. Empty means every ticket is worked
	// in Cfg.Repo.
	Repos RepoSet
	// AgentID, when the work was asked of an agent from Raenil, is recorded on
	// each run so the agent's page can show it.
	AgentID string
	// Instructions are the agent's own, given ahead of the ticket on every
	// work run, as a chat turn gets them.
	Instructions string
	// MCP gives a work run Raenil's tools — ask_user, so a worker that needs a
	// decision asks for it instead of guessing.
	MCP *MCPServer
	// HostName names this machine on run records; empty means its hostname.
	// A runner host sets its own name, so its runs can be matched to it.
	HostName string
	// labelGroups maps a group name to its id, resolved per ticket so labels can
	// be matched by group membership rather than by how they happen to be named.
	labelGroups map[string]string
	// Log receives progress lines. Nil discards them.
	Log func(string, ...any)
}

func (o *Orchestrator) logf(format string, args ...any) {
	if o.Log != nil {
		o.Log(format, args...)
	}
}

// forTicket resolves everything that varies per ticket — which workspace owns
// it and which repository it is worked in — and returns an orchestrator already
// pointed at both, so the rest of the flow needs no special cases.
func (o *Orchestrator) forTicket(ctx context.Context, ref string) (*Orchestrator, models.Issue, error) {
	var issue models.Issue
	rc, err := o.Raenil.Scoped(ctx, ref)
	if err != nil {
		return nil, issue, err
	}
	issue, err = rc.Issue(ctx, ref)
	if err != nil {
		return nil, issue, fmt.Errorf("fetch issue %s: %w", ref, err)
	}
	groups, gerr := rc.LabelGroups(ctx)
	if gerr != nil {
		// Not fatal: a "group:value" label name still matches without them.
		o.logf("warning: could not read label groups, falling back to label names: %v", gerr)
		groups = map[string]string{}
	}
	repo := o.Cfg.Repo
	if len(o.Repos) > 0 {
		repo, err = o.Repos.RepoFor(issue, groups[RepoGroup], o.Cfg.Repo)
		if err != nil {
			return nil, issue, err
		}
	}
	scoped := *o
	scoped.Raenil = rc
	scoped.Cfg.Repo = repo
	scoped.labelGroups = groups
	return &scoped, issue, nil
}

// AttemptSpec describes one attempt. A repair attempt carries the previous
// verdict and evidence so the worker is told exactly what refused it.
type AttemptSpec struct {
	Attempt int
	// Slot names this attempt's branch, worktree and run directory. Zero means
	// Attempt. Work sets it past the ticket's earlier runs, so running a
	// handed-back ticket again never collides with the branch it kept.
	Slot int
	// SessionID continues a previous attempt's session, keeping its context.
	SessionID string
	// Model overrides the configured model, which is how escalation works.
	Model string
	// PriorVerdict and PriorEvidence turn the prompt into a repair brief.
	PriorVerdict  *Verdict
	PriorEvidence []Evidence
	// FromBranch continues a failed attempt: the new worktree starts from its
	// branch, so the work it did is there to fix rather than redo. FromBase is
	// where the ticket's work began, so the checks and the review still see
	// the whole change. Empty, or a branch that is gone, starts fresh.
	FromBranch string
	FromBase   string
}

// RunTicket runs a single attempt. It returns the verdict even when the attempt
// failed: a failing ticket is a result, not an error. An error means the
// orchestrator itself could not do its job.
func (o *Orchestrator) RunTicket(ctx context.Context, ref string, attempt int) (Verdict, error) {
	v, _, err := o.RunAttempt(ctx, ref, AttemptSpec{Attempt: attempt})
	return v, err
}

// RunAttempt runs one attempt and returns its verdict and evidence.
func (o *Orchestrator) RunAttempt(ctx context.Context, ref string, spec AttemptSpec) (Verdict, []Evidence, error) {
	attempt := spec.Attempt
	if attempt < 1 {
		attempt = 1
	}
	slot := spec.Slot
	if slot < 1 {
		slot = attempt
	}
	var v Verdict

	// 1. Read the ticket and its contract, in its own workspace and repository.
	scoped, issue, err := o.forTicket(ctx, ref)
	if err != nil {
		return v, nil, err
	}
	o = scoped
	cfg := o.Cfg.withDefaults()

	stored, err := o.Raenil.Criteria(ctx, issue.ID)
	if err != nil {
		return v, nil, fmt.Errorf("fetch criteria: %w", err)
	}
	criteria, err := ParseCriteria(stored)
	if err != nil {
		// A criterion that cannot be read is a gate that cannot gate. Refuse to
		// start rather than run a ticket whose contract is unreadable.
		return v, nil, fmt.Errorf("issue %s has an unusable checklist: %w", issue.Key, err)
	}
	if len(criteria) == 0 {
		return v, nil, fmt.Errorf("issue %s has no done-when criteria — nothing to verify against", issue.Key)
	}
	if !anyGating(criteria) {
		return v, nil, fmt.Errorf("issue %s has no deterministic or policy criteria — "+
			"nothing an orchestrator can decide", issue.Key)
	}
	o.logf("ticket %s: %s (%d criteria)", issue.Key, issue.Title, len(criteria))

	// 2. Pick the runner and prove it can work — BEFORE claiming the ticket.
	//
	// Selecting the runner at the point of use meant a misconfigured agent got
	// discovered only after the ticket had been moved to In Progress and a
	// worktree cut, stranding the ticket with nothing to show. Whether the work
	// is even possible has to be settled before anything is taken.
	model := cfg.Model
	if spec.Model != "" {
		model = spec.Model
	}
	runner := o.Runner
	if len(o.Runners) > 0 {
		chosen, rerr := o.Runners.RunnerFor(issue, o.labelGroups[RunnerGroup], o.Runner)
		if rerr != nil {
			return v, nil, rerr
		}
		runner = chosen
	}
	if runner == nil {
		return v, nil, fmt.Errorf("no runner configured")
	}
	if err := runnerReady(ctx, runner, model); err != nil {
		return v, nil, fmt.Errorf("not starting %s: %w", issue.Key, err)
	}

	// 3. Claim it.
	branch := fmt.Sprintf("ticket/%s-attempt-%d", strings.ToLower(issue.Key), slot)
	if err := o.Raenil.SetState(ctx, issue.ID, cfg.StateInProgress); err != nil {
		return v, nil, fmt.Errorf("claim issue: %w", err)
	}
	// From here on the ticket is ours; anything that stops us before work
	// begins has to give it back rather than leave it parked In Progress.
	releaseClaim := func(why string) {
		name, nerr := o.Raenil.StateName(context.WithoutCancel(ctx), issue.StateID)
		if nerr != nil || name == "" {
			o.logf("warning: could not work out where %s came from, leaving it In Progress: %v", issue.Key, nerr)
			return
		}
		if err := o.Raenil.SetState(context.WithoutCancel(ctx), issue.ID, name); err != nil {
			o.logf("warning: could not return %s to %s: %v", issue.Key, name, err)
			return
		}
		o.logf("returned %s to %s (%s)", issue.Key, name, why)
	}
	if err := o.Raenil.SetDev(ctx, issue.ID, branch, ""); err != nil {
		o.logf("warning: could not record branch: %v", err)
	}

	// 4. Isolate. The worker writes freely in here and nowhere else.
	runDir, err := NewRunDir(cfg.RunRoot, issue.Key, slot)
	if err != nil {
		releaseClaim("could not create the run directory")
		return v, nil, err
	}
	wtPath := filepath.Join(cfg.RunRoot, "worktrees", fmt.Sprintf("%s-%d", issue.Key, slot))
	// ABSOLUTE, against the repo, and the reason is a silent failure worth
	// keeping written down.
	//
	// RunRoot defaults to ".orchestrator", so this path is relative. Every
	// git call below is fine with that — `git -C <repo> worktree add` and the
	// later commands resolve it against the REPO, which is where the worktree
	// really lands. But the same string is handed to the worker as its `Cwd`,
	// and a runner resolves it against ITS OWN working directory. Running the
	// orchestrator from anywhere other than the repo therefore pointed the
	// worker at a directory that did not exist.
	//
	// opencode does not refuse that. It creates the session, accepts the
	// prompt with a 204, records the user message — and the assistant comes
	// back with zero parts, zero tokens and no error, so the poll loop spins
	// to the timeout and the ticket looks like a slow agent rather than a
	// broken one. Two models and two projects burned an hour each that way.
	//
	// `os.RemoveAll` below had the same bug with worse consequences: a
	// relative path there deletes whatever sits at that name under the
	// orchestrator's own cwd.
	if !filepath.IsAbs(wtPath) {
		abs, aerr := filepath.Abs(filepath.Join(cfg.Repo, wtPath))
		if aerr != nil {
			releaseClaim("could not resolve the worktree path")
			return v, nil, fmt.Errorf("resolve worktree path: %w", aerr)
		}
		wtPath = abs
	}
	_ = os.RemoveAll(wtPath)
	start, continuing := cfg.BaseRef, false
	if spec.FromBranch != "" {
		if _, err := git(ctx, cfg.Repo, "rev-parse", "--verify", "--quiet", spec.FromBranch+"^{commit}"); err == nil {
			start, continuing = spec.FromBranch, true
		}
	}
	wt, err := AddWorktree(ctx, cfg.Repo, wtPath, branch, start)
	if err != nil {
		releaseClaim("could not create a worktree")
		return v, nil, fmt.Errorf("create worktree: %w", err)
	}
	if continuing {
		// The ticket's work began where the first attempt did, not at the
		// last attempt's commit: keep that as the base the checks diff from.
		base := spec.FromBase
		if base == "" {
			if head, err := git(ctx, cfg.Repo, "rev-parse", cfg.BaseRef); err == nil {
				if mb, err := git(ctx, cfg.Repo, "merge-base", spec.FromBranch, strings.TrimSpace(head)); err == nil {
					base = strings.TrimSpace(mb)
				}
			}
		}
		if base != "" {
			_ = setWorktreeBase(ctx, wtPath, base)
		}
		o.logf("continuing from %s", spec.FromBranch)
	}
	// Build on the tickets this one waits on that are still in review: their
	// work is not on the main line yet.
	if branches, err := o.reviewBranches(ctx, issue.Key); err != nil {
		_ = wt.Remove(context.WithoutCancel(ctx))
		releaseClaim("could not read its blockers")
		return v, nil, fmt.Errorf("not starting %s: %w", issue.Key, err)
	} else if len(branches) > 0 {
		o.logf("building on %s", strings.Join(branches, ", "))
		if err := MergeBlockerBranches(ctx, wtPath, branches); err != nil {
			_ = wt.Remove(context.WithoutCancel(ctx))
			releaseClaim("its blockers' work does not combine")
			return v, nil, fmt.Errorf("not starting %s: %w", issue.Key, err)
		}
	}
	keepBranch := false
	defer func() {
		// Handoff keeps the checkout itself, not just the branch: a reviewer
		// needs somewhere to read the code, run the checks and make a fix.
		// Detach would remove the directory and leave them with a branch name.
		if cfg.Handoff {
			return
		}
		if keepBranch {
			_ = wt.Detach(context.WithoutCancel(ctx))
			return
		}
		_ = wt.Remove(context.WithoutCancel(ctx))
	}()

	// 5. Hand the work over.
	prompt := BuildPrompt(issue, criteria)
	if spec.PriorVerdict != nil {
		prompt = BuildRepairPrompt(issue, criteria, *spec.PriorVerdict, spec.PriorEvidence)
		if continuing {
			prompt += "\n\n## Your previous attempt is in this worktree\n\nIt was committed on " + spec.FromBranch +
				" and this worktree starts from it. Fix what failed; do not start over.\n"
		}
	}
	if in := strings.TrimSpace(o.Instructions); in != "" {
		prompt = "## Your instructions\n\n" + in + "\n\n" + prompt
	}
	// Answers the user already gave on this ticket are part of the brief: a
	// worker that asked must not be sent back to work without them.
	if its, err := o.Raenil.Interactions(ctx, issue.ID); err == nil {
		if d := decisionsMade(its); d != "" {
			prompt += "\n\n## Decisions already made\n\n" + d
		}
	}
	if o.MCP != nil {
		prompt += "\n\n## When you need a decision\n\nIf you cannot finish without a decision only the user can make, " +
			"call the Raenil tool ask_user on " + issue.Key + " with a few concrete options, then end your turn. " +
			"Do not guess, and do not stop for anything you can decide yourself.\n\n" +
			"Asking is rare: the ticket is the spec, and the user is not watching. When the ticket leaves a " +
			"choice open, pick what fits the existing code best, say so in your final summary, and carry on.\n\n" +
			"If the work needs code, tables or files another ticket delivers and that ticket is not in place, " +
			"call the Raenil tool add_blocker on " + issue.Key + " naming that ticket and what you need from it, then " +
			"end your turn. Do not build a stand-in for the other ticket's work; this ticket starts again on its " +
			"own, on top of that work, once it is ready.\n"
	}
	if err := os.WriteFile(runDir.File("context.md"), []byte(prompt), 0o644); err != nil {
		return v, nil, err
	}
	// Record the attempt on the ticket before it starts, so it is visible while
	// it runs. A tracker that cannot take the record does not stop the work.
	host := o.HostName
	if host == "" {
		host, _ = os.Hostname()
	}
	runRec, recErr := o.Raenil.StartRun(ctx, RunStartReq{IssueID: issue.ID, AgentID: o.AgentID, Kind: "work",
		Runner: runner.Name(), Model: effectiveModel(runner, model), Attempt: slot, Host: host})
	if recErr != nil {
		o.logf("warning: could not record the run: %v", recErr)
	}
	var (
		res           RunResult
		runErr        error
		verdictStatus string
		patchText     string
	)
	defer func() {
		if runRec.ID != "" {
			o.finishRun(context.WithoutCancel(ctx), runRec.ID, runner.Name(), runDir.File("worker.log"), res, runErr, verdictStatus, patchText)
		}
	}()

	runStart := time.Now()
	o.logf("running %s on %s", runner.Name(), effectiveModel(runner, model))
	stopStream := streamLog(ctx, o.Raenil, runRec.ID, runDir.File("worker.log"), runner.Name())
	res, runErr = runner.Run(ctx, RunRequest{
		Prompt:    prompt,
		Cwd:       wtPath,
		Model:     model,
		Timeout:   cfg.Timeout,
		LogPath:   runDir.File("worker.log"),
		SessionID: spec.SessionID,
		MCP:       o.MCP,
	})
	stopStream()
	if runErr != nil {
		o.logf("runner error: %v", runErr)
	}
	// Questions the agent asked on the ticket during this run stop the
	// attempts: they wait for the answers, not for another try.
	if o.AgentID != "" {
		res.Questions = append(res.Questions, o.questionsAskedSince(ctx, issue.ID, runStart)...)
	}
	if res.CostUnknown {
		o.logf("worker finished: exit=%d aborted=%v cost=UNREPORTED (%s bills against a subscription) tokens=%d",
			res.Exit, res.Aborted, runner.Name(), res.Tokens)
	} else {
		o.logf("worker finished: exit=%d aborted=%v cost=$%.4f tokens=%d",
			res.Exit, res.Aborted, res.CostUSD, res.Tokens)
	}

	// A question the worker asked is the escalation payload, not a stall.
	if len(res.Questions) > 0 {
		o.logf("worker asked %d question(s) — escalating", len(res.Questions))
		_ = o.Raenil.Comment(ctx, issue.ID, "The worker asked a question it could not proceed without:\n\n> "+
			strings.Join(res.Questions, "\n> ")+"\n\nAttempt "+fmt.Sprint(attempt)+" stopped here.")
	}

	// The agent may use git checkout; it must hand the worktree back on the
	// ticket's branch, or its work would be committed somewhere else.
	if err := onTicketBranch(ctx, wtPath, branch); err != nil {
		return v, nil, err
	}

	// 6. See what it actually did.
	// Diff from where the ticket's work began: after blockers were merged
	// in, or where a continued attempt's work started.
	diffBase := cfg.BaseRef
	if b := WorktreeBase(ctx, wtPath); b != "" {
		diffBase = b
	}
	diff, err := StageAndDiff(ctx, wtPath, diffBase)
	if err != nil {
		return v, nil, fmt.Errorf("read diff: %w", err)
	}
	if patch, derr := gitRaw(ctx, wtPath, "diff", "--cached", diffBase); derr == nil {
		_ = os.WriteFile(runDir.File("diff.patch"), []byte(patch), 0o644)
		patchText = redact(patch)
	}

	// 7. Decide, ticking each criterion the moment it is met.
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
			if !e.Pass {
				return
			}
			c, ok := byIndex[e.CriterionIndex]
			if !ok || c.ID == "" {
				return
			}
			ref := fmt.Sprintf("%s/evidence.json#%d", runDir.Path(), e.CriterionIndex)
			if err := o.Raenil.TickCriterion(ctx, c.ID, true, ref); err != nil {
				o.logf("warning: could not tick criterion %d: %v", e.CriterionIndex+1, err)
			}
		},
	}
	evidence, err := ev.Evaluate(ctx, criteria, diff)
	if err != nil {
		return v, nil, fmt.Errorf("evaluate criteria: %w", err)
	}
	for _, e := range evidence {
		o.logf("  [%s] %s: pass=%v %s%s", e.Kind, e.CriterionText, e.Pass, e.Detail, e.Err)
	}

	v = Summarise(issue.Key, attempt, criteria, evidence, diff)
	verdictStatus = string(v.Status)
	v.Runner, v.Model = runner.Name(), model
	v.CostUSD += res.CostUSD // Summarise already counted what evaluation spent
	v.DurationS = int64(res.Duration.Seconds())
	v.SessionID, v.Questions, v.DeniedTools = res.SessionID, res.Questions, res.DeniedTools
	v.Branch, v.Base = branch, WorktreeBase(ctx, wtPath)
	if o.AgentID != "" {
		if blockers, err := o.Raenil.Blockers(ctx, issue.Key); err == nil {
			for _, b := range blockers {
				if !b.Done {
					v.WaitingOn = append(v.WaitingOn, b.Key)
				}
			}
		}
	}
	if res.Aborted {
		v.Status, v.Next, v.Blocked = StatusBlocked, "escalate", "worker timed out"
	}

	// 8. Record the outcome.
	//
	// In handoff mode the orchestrator stops here regardless of the verdict: the
	// worker's work is committed so a reviewer can see exactly what it produced,
	// the worktree is kept so they can work in it, and nothing moves on the
	// tracker. A reviewer arriving to find the worktree deleted has nothing to
	// review, and a verdict already filed as In Review pre-empts them.
	if cfg.Handoff {
		sha, cerr := Commit(ctx, wtPath, fmt.Sprintf("%s: %s", issue.Key, issue.Title))
		switch {
		case cerr != nil:
			o.logf("worker changed nothing to commit (%v)", cerr)
		default:
			_ = o.Raenil.LinkCommit(ctx, issue.ID, sha, issue.Title+" (worker)")
			o.logf("worker commit %s on %s", sha[:min(8, len(sha))], branch)
		}
		keepBranch = true
		o.logf("verdict: %s — handing over, worktree kept at %s", v.Status, wtPath)
		if err := runDir.WriteVerdict(v); err != nil {
			return v, evidence, err
		}
		return v, evidence, nil
	}

	if v.Status == StatusPassed {
		sha, cerr := Commit(ctx, wtPath, fmt.Sprintf("%s: %s", issue.Key, issue.Title))
		if cerr != nil {
			o.logf("warning: commit failed: %v", cerr)
		} else {
			keepBranch = true
			_ = o.Raenil.LinkCommit(ctx, issue.ID, sha, issue.Title)
			o.logf("committed %s on %s", sha[:min(8, len(sha))], branch)
		}
		if err := o.Raenil.SetState(ctx, issue.ID, cfg.StateInReview); err != nil {
			o.logf("warning: could not move to %s: %v", cfg.StateInReview, err)
		}
		o.logf("verdict: passed → %s", cfg.StateInReview)
	} else {
		_ = o.Raenil.Comment(ctx, issue.ID, failureComment(v, evidence))
		o.logf("verdict: %s (%s)", v.Status, v.Next)
	}

	if err := runDir.WriteVerdict(v); err != nil {
		return v, evidence, err
	}
	return v, evidence, nil
}

func anyGating(cs []ParsedCriterion) bool {
	for _, c := range cs {
		if c.Gating() {
			return true
		}
	}
	return false
}

// BuildPrompt renders the ticket as a near-executable spec. The worker is shown
// the exact commands and rules it will be judged by, because a worker that can
// see the contract can satisfy it, and one that cannot is guessing.
func BuildPrompt(issue models.Issue, criteria []ParsedCriterion) string {
	return buildPrompt(issue, criteria, true)
}

// BuildPromptWithoutGuards renders the same ticket with the policy criteria and
// the anti-cheating rule removed, so the worker cannot see that weakening the
// tests is detected.
//
// This exists only to measure whether disclosure is what produces honest
// behaviour. It must never be used to drive real work: a worker that cannot see
// the contract is being judged on rules it was never told.
func BuildPromptWithoutGuards(issue models.Issue, criteria []ParsedCriterion) string {
	return buildPrompt(issue, criteria, false)
}

func buildPrompt(issue models.Issue, criteria []ParsedCriterion, showGuards bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n\n", issue.Key, issue.Title)
	if strings.TrimSpace(issue.DescriptionMD) != "" {
		b.WriteString(issue.DescriptionMD)
		b.WriteString("\n\n")
	}

	b.WriteString("## Acceptance criteria\n\n")
	b.WriteString("Your work is judged mechanically against the list below. " +
		"Nothing you assert about your own work counts; only these checks do.\n\n")
	for _, c := range criteria {
		if !showGuards && c.Kind == models.CriterionPolicy {
			continue
		}
		switch c.Kind {
		case models.CriterionDeterministic:
			fmt.Fprintf(&b, "%d. %s\n   Verified by running: `%s`\n", c.Index+1, c.Text, c.Deterministic.Cmd)
			if c.Deterministic.ExpectExit != nil && *c.Deterministic.ExpectExit != 0 {
				fmt.Fprintf(&b, "   Must exit %d.\n", *c.Deterministic.ExpectExit)
			}
			if c.Deterministic.Advisory {
				b.WriteString("   Advisory: a model's opinion, shown to the reviewer. It does not fail the run — " +
					"meet the requirement it describes, but do not change correct code just to please it.\n")
			}
		case models.CriterionPolicy:
			fmt.Fprintf(&b, "%d. %s\n   Enforced as policy `%s`", c.Index+1, c.Text, c.Policy.Policy)
			if len(c.Policy.Args) > 0 {
				fmt.Fprintf(&b, " (%s)", strings.Join(c.Policy.Args, ", "))
			}
			b.WriteString("\n")
		case models.CriterionJudgment:
			fmt.Fprintf(&b, "%d. %s\n   Reviewed for: %s\n", c.Index+1, c.Text, c.Judgment.Prompt)
		default:
			fmt.Fprintf(&b, "%d. %s\n   (checked by a human)\n", c.Index+1, c.Text)
		}
	}

	b.WriteString("\n## Rules\n\n")
	b.WriteString("- Work only inside this directory. It is an isolated worktree.\n")
	if showGuards {
		b.WriteString("- Do not weaken tests. Deleting a test, or adding a skip/only marker, " +
			"is detected and fails the ticket outright.\n")
	}
	b.WriteString("- Do not commit. The orchestrator commits if the checks pass.\n")
	b.WriteString("- If something is genuinely ambiguous, say so rather than guessing; " +
		"the ticket will go back to a human.\n")
	return b.String()
}

// failureComment explains to a human exactly what refused the attempt.
func failureComment(v Verdict, ev []Evidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Attempt %d did not pass (%s).\n\n", v.Attempt, v.Status)
	if v.Blocked != "" {
		fmt.Fprintf(&b, "Blocked: %s\n\n", v.Blocked)
	}
	byIndex := map[int]Evidence{}
	for _, e := range ev {
		byIndex[e.CriterionIndex] = e
	}
	for _, i := range v.Failed {
		e := byIndex[i]
		detail := e.Detail
		if e.Err != "" {
			detail = e.Err
		}
		fmt.Fprintf(&b, "- **%s** — %s\n", e.CriterionText, strings.TrimSpace(detail))
	}
	return b.String()
}

// gitRaw exposes git output to callers in this package.
func gitRaw(ctx context.Context, dir string, args ...string) (string, error) {
	return git(ctx, dir, args...)
}

// Commit commits everything staged in a worktree and returns the new SHA.
func Commit(ctx context.Context, dir, message string) (string, error) {
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := git(ctx, dir, "-c", "user.email=orchestrator@raenil.local",
		"-c", "user.name=Raenil Orchestrator", "commit", "-m", message); err != nil {
		return "", err
	}
	sha, err := git(ctx, dir, "rev-parse", "HEAD")
	return strings.TrimSpace(sha), err
}

// finishRun closes the run record with what the attempt produced. The log tail
// is rendered and redacted here, on the machine that ran it, before it leaves.
func (o *Orchestrator) finishRun(ctx context.Context, runID, runner, logPath string, res RunResult, runErr error, verdict, diff string) {
	out := RunOutcome{
		Status:      runOutcomeStatus(res, runErr),
		Verdict:     verdict,
		SessionID:   res.SessionID,
		AgentError:  res.AgentError,
		CostUSD:     res.CostUSD,
		NotionalUSD: res.NotionalCostUSD,
		Billing:     res.Billing,
		DeniedTools: res.DeniedTools,
		LogTail:     runLogTail(logPath, runner),
		Diff:        diff,
		Tokens: map[string]int{
			"input": res.Usage.Input, "cacheRead": res.Usage.CacheRead,
			"cacheCreation": res.Usage.CacheCreation, "output": res.Usage.Output, "total": res.Tokens,
		},
	}
	if runErr != nil {
		out.AgentError = strings.TrimSpace(runErr.Error() + " " + out.AgentError)
	}
	if runErr == nil || res.Exit != 0 {
		exit := res.Exit
		out.ExitCode = &exit
	}
	if err := o.Raenil.FinishRun(ctx, runID, out); err != nil {
		o.logf("warning: could not close the run record: %v", err)
	}
}

// runOutcomeStatus says how the agent's attempt itself ended — not whether the
// criteria passed, which is the verdict.
func runOutcomeStatus(res RunResult, runErr error) string {
	switch {
	case res.Aborted:
		return "aborted"
	case runErr != nil, res.Exit != 0, res.AgentError != "":
		return "failed"
	default:
		return "succeeded"
	}
}

// questionsAskedSince returns the questions this orchestrator's agent posted
// on a ticket, still open, since a moment.
func (o *Orchestrator) questionsAskedSince(ctx context.Context, issueID string, since time.Time) []string {
	its, err := o.Raenil.Interactions(ctx, issueID)
	if err != nil {
		return nil
	}
	var out []string
	for _, it := range its {
		if it.Kind != "questions" || it.Status != "open" || it.AgentID == nil || *it.AgentID != o.AgentID || it.CreatedAt.Before(since) {
			continue
		}
		var p struct {
			Questions []models.Question `json:"questions"`
		}
		_ = json.Unmarshal(it.Payload, &p)
		for _, q := range p.Questions {
			out = append(out, q.Text)
		}
	}
	return out
}

// decisionsMade lists the questions answered on a ticket, question → answer.
func decisionsMade(its []models.Interaction) string {
	var b strings.Builder
	for _, it := range its {
		if it.Kind == "questions" && it.Status == "answered" {
			b.WriteString(strings.TrimPrefix(renderResponse(it), "**User answered:**\n"))
		}
	}
	return strings.TrimSpace(b.String())
}

// reviewBranches are the branches of the tickets an issue waits on: done
// enough to build on — In Review, or Done — whether or not anyone has merged
// them yet. MergeBlockerBranches skips any already on the main line, so a
// ticket marked Done before its branch was merged still hands its work on.
// A blocker still open is the server's to refuse.
func (o *Orchestrator) reviewBranches(ctx context.Context, key string) ([]string, error) {
	blockers, err := o.Raenil.Blockers(ctx, key)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, b := range blockers {
		if !b.Done {
			return nil, fmt.Errorf("blocked by %s (%s)", b.Key, b.State)
		}
		bi, err := o.Raenil.Issue(ctx, b.Key)
		if err != nil {
			return nil, err
		}
		if bi.GitBranch == nil || *bi.GitBranch == "" {
			// Done by hand with no run behind it: nothing to build on.
			if b.State == "In Review" {
				return nil, fmt.Errorf("%s is In Review but has no branch recorded to build on", b.Key)
			}
			continue
		}
		out = append(out, *bi.GitBranch)
	}
	return out, nil
}

// onTicketBranch checks a worktree is still on the ticket's branch.
func onTicketBranch(ctx context.Context, dir, branch string) error {
	cur, err := git(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	if cur = strings.TrimSpace(cur); cur != branch {
		return fmt.Errorf("the agent left the worktree on %q instead of %q; nothing was committed — "+
			"switch it back with git checkout %s in %s, then Verify", cur, branch, branch, dir)
	}
	return nil
}
