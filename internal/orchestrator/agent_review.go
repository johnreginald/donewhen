package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"raenil/internal/models"
)

// Two agent reviews wrap a build, the way a team would:
//
//   - The ticket reviewer qualifies a ticket before anyone builds it: can
//     every check pass with correct code, does it clash with other tickets,
//     is anything missing or too big. A blocking finding sends the ticket
//     back to the spec instead of spending attempts on it.
//   - The code reviewer reads the change after its checks pass — a different
//     vendor than the builder, read-only, fresh context. Blocking findings go
//     back to the builder's session; the review guide goes to the user.
//
// Both answer with one JSON object, stored on the ticket.

// reviewerOrder is who reviews, in order of preference, skipping the builder.
var reviewerOrder = []string{"codex", "claude", "antigravity", "opencode"}

// PickReviewer chooses a reviewer from the pool: a different harness than the
// builder when there is one, else the builder's own.
func PickReviewer(builder string, pool RunnerSet) (Runner, string) {
	for _, name := range reviewerOrder {
		if name == builder {
			continue
		}
		if r, ok := pool[name]; ok {
			return r, name
		}
	}
	if r, ok := pool[builder]; ok {
		return r, builder
	}
	return nil, ""
}

// ReviewTimeout bounds one review.
const ReviewTimeout = 15 * time.Minute

// reviewAnswer is what a reviewer replies with.
type reviewAnswer struct {
	Verdict  string                 `json:"verdict"`
	Summary  string                 `json:"summary"`
	Findings []models.ReviewFinding `json:"findings"`
	Guide    string                 `json:"guide"`
}

// blocking reports whether any finding must be fixed first.
func blocking(fs []models.ReviewFinding) []models.ReviewFinding {
	var out []models.ReviewFinding
	for _, f := range fs {
		if strings.EqualFold(f.Severity, "blocking") {
			out = append(out, f)
		}
	}
	return out
}

// parseReviewAnswer finds the JSON object in a reviewer's reply. The verdict
// follows the findings, whatever the reviewer called it: changes only when
// something blocks.
func parseReviewAnswer(text string) (reviewAnswer, error) {
	var a reviewAnswer
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j <= i {
		return a, errors.New("the reviewer did not answer with JSON")
	}
	if err := json.Unmarshal([]byte(text[i:j+1]), &a); err != nil {
		return a, fmt.Errorf("the reviewer's JSON does not parse: %w", err)
	}
	for k := range a.Findings {
		if !strings.EqualFold(a.Findings[k].Severity, "blocking") {
			a.Findings[k].Severity = "minor"
		} else {
			a.Findings[k].Severity = "blocking"
		}
	}
	a.Verdict = "pass"
	if len(blocking(a.Findings)) > 0 {
		a.Verdict = "changes"
	}
	return a, nil
}

// askReviewer runs a read-only review and parses its answer.
func askReviewer(ctx context.Context, reviewer Runner, model, cwd, prompt, logPath string) (reviewAnswer, error) {
	res, err := reviewer.Run(ctx, RunRequest{Prompt: prompt, Cwd: cwd, Model: model, ReadOnlyTools: true,
		Timeout: ReviewTimeout, LogPath: logPath})
	if err != nil {
		return reviewAnswer{}, err
	}
	if res.Answer == "" && res.AgentError != "" {
		return reviewAnswer{}, errors.New(res.AgentError)
	}
	return parseReviewAnswer(res.Answer)
}

// reviewerModel is the model a reviewer runs with: its own default.
func reviewerModel(name string) string {
	switch name {
	case "claude", "codex", "antigravity":
		return name + "/default"
	}
	return ""
}

// CodeReview has a reviewer read a ticket's change in its worktree and
// records the review.
func (o *Orchestrator) CodeReview(ctx context.Context, issue models.Issue, criteria []ParsedCriterion,
	wtPath, diff, builder string, reviewer Runner, reviewerName string, round int, logPath string) (models.Review, error) {
	if len(diff) > 120*1024 {
		diff = diff[:120*1024] + "\n… (diff cut at 120 KB; read the files for the rest)\n"
	}
	var checks strings.Builder
	for _, c := range criteria {
		fmt.Fprintf(&checks, "- %s\n", c.Text)
	}
	prompt := "You are the code reviewer for ticket " + issue.Key + ". Another agent built it; you did not.\n\n" +
		"Read the ticket, the diff, and any code you need in this worktree. Read-only: do not change any file.\n\n" +
		"Check:\n1. It does what the ticket asks, and nothing outside it.\n" +
		"2. The tests are real: they test the behaviour, none were weakened, and none are gamed to pass.\n" +
		"3. Bugs, security problems, risks.\n4. What the tests do not prove.\n\n" +
		"Blocking = must be fixed before a person reviews it: wrong behaviour, a missing requirement, a weakened or " +
		"gamed test, a real bug, a security problem. Minor = worth knowing, not worth blocking on.\n\n" +
		"Then write a review guide for the person who reviews next, in plain short English: what changed (bullets); " +
		"the key code paths with file:line; choices the builder made where the ticket left room; what the tests prove " +
		"and what they don't; where it could fail.\n\n" +
		"Reply with ONLY one JSON object:\n" +
		`{"verdict":"pass"|"changes","summary":"one or two sentences","findings":[{"severity":"blocking"|"minor",` +
		`"file":"path","line":12,"issue":"what is wrong","fix":"what to do"}],"guide":"markdown"}` + "\n\n" +
		"## Ticket\n\n" + issue.DescriptionMD + "\n\n## Checks (all passing)\n\n" + checks.String() +
		"\n## Diff\n\n```diff\n" + diff + "\n```\n"
	a, err := askReviewer(ctx, reviewer, reviewerModel(reviewerName), wtPath, prompt, logPath)
	if err != nil {
		return models.Review{}, fmt.Errorf("code review by %s: %w", reviewerName, err)
	}
	rv := models.Review{IssueID: issue.ID, Kind: "code", Reviewer: reviewerName, Builder: builder,
		Verdict: a.Verdict, Summary: a.Summary, Findings: a.Findings, GuideMD: a.Guide, Round: round}
	if err := o.Raenil.SaveReview(ctx, issue.ID, rv); err != nil {
		o.logf("warning: could not record the code review: %v", err)
	}
	o.logf("code review by %s (round %d): %s — %d blocking, %d minor", reviewerName, round, a.Verdict,
		len(blocking(a.Findings)), len(a.Findings)-len(blocking(a.Findings)))
	return rv, nil
}

// ticketHash identifies what a ticket review saw: the description and the
// checklist. A ticket edited since needs qualifying again.
func ticketHash(issue models.Issue, stored []models.Criterion) string {
	h := sha256.New()
	h.Write([]byte(issue.DescriptionMD))
	for _, c := range stored {
		fmt.Fprintf(h, "\x00%s\x00%s\x00%s", c.Body, c.Kind, string(c.CheckSpec))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// QualifyTicket has a reviewer qualify a ticket before it is built. A ticket
// already qualified as it stands is not reviewed again.
func (o *Orchestrator) QualifyTicket(ctx context.Context, ref string, reviewer Runner, reviewerName string) (models.Review, error) {
	scoped, issue, err := o.forTicket(ctx, ref)
	if err != nil {
		return models.Review{}, err
	}
	o = scoped
	stored, err := o.Raenil.Criteria(ctx, issue.ID)
	if err != nil {
		return models.Review{}, err
	}
	hash := ticketHash(issue, stored)
	if prior, err := o.Raenil.Reviews(ctx, issue.ID); err == nil {
		for _, p := range prior {
			if p.Kind == "ticket" {
				if p.ContentHash == hash && p.Verdict == "pass" {
					o.logf("%s is qualified as it stands", issue.Key)
					return p, nil
				}
				break
			}
		}
	}

	var checks strings.Builder
	for _, c := range stored {
		spec := strings.TrimSpace(string(c.CheckSpec))
		if spec == "" || spec == "null" {
			spec = "(no check)"
		}
		fmt.Fprintf(&checks, "- [%s] %s\n  check: %s\n", c.Kind, c.Body, spec)
	}
	siblings := o.epicSiblings(ctx, issue)

	prompt := "You qualify ticket " + issue.Key + " before an agent builds it. Read it, its checks, and the other " +
		"tickets in its epic. Look at the repository (read-only) where it helps.\n\n" +
		"Blocking — the ticket must go back to its author:\n" +
		"- a check that cannot pass with correct code, or that tests the wrong thing;\n" +
		"- requirements that contradict each other, or another ticket;\n" +
		"- a test or rule that will break when a later ticket does its job (e.g. a hard-coded count);\n" +
		"- information the builder needs and cannot decide;\n" +
		"- work that depends on something no ticket delivers.\n" +
		"Minor — worth fixing, not worth stopping for: clarity, format, small gaps.\n\n" +
		"Size is deliberate: one ticket is one feature end to end (database, API and tests together), and every " +
		"screen is its own ticket. Never block on size; mention it as minor only if a part could clearly stand alone.\n\n" +
		"Reply with ONLY one JSON object:\n" +
		`{"verdict":"pass"|"changes","summary":"one or two sentences","findings":[{"severity":"blocking"|"minor",` +
		`"issue":"what is wrong","fix":"what to change"}]}` + "\n\n" +
		"## Ticket " + issue.Key + ": " + issue.Title + "\n\n" + issue.DescriptionMD +
		"\n\n## Checks\n\n" + checks.String() + "\n## Other tickets in the epic\n\n" + siblings
	repo := o.Cfg.withDefaults().Repo
	a, err := askReviewer(ctx, reviewer, reviewerModel(reviewerName), repo, prompt, "")
	if err != nil {
		return models.Review{}, fmt.Errorf("ticket review by %s: %w", reviewerName, err)
	}
	rv := models.Review{IssueID: issue.ID, Kind: "ticket", Reviewer: reviewerName, Verdict: a.Verdict,
		Summary: a.Summary, Findings: a.Findings, ContentHash: hash}
	if err := o.Raenil.SaveReview(ctx, issue.ID, rv); err != nil {
		o.logf("warning: could not record the ticket review: %v", err)
	}
	o.logf("ticket review of %s by %s: %s — %d blocking", issue.Key, reviewerName, a.Verdict, len(blocking(a.Findings)))
	return rv, nil
}

// epicSiblings lists the other tickets in an issue's epic: key, title, and
// the first lines of each, enough to spot a clash without the whole text.
func (o *Orchestrator) epicSiblings(ctx context.Context, issue models.Issue) string {
	if issue.ProjectID == nil || *issue.ProjectID == "" {
		return "(no epic)\n"
	}
	var list []models.Issue
	if err := o.Raenil.do(ctx, http.MethodGet, "/api/issues?project="+url.QueryEscape(*issue.ProjectID)+"&limit=200", nil, &list); err != nil {
		return "(could not list them)\n"
	}
	var b strings.Builder
	for _, s := range list {
		if s.ID == issue.ID {
			continue
		}
		fmt.Fprintf(&b, "### %s: %s\n%s\n\n", s.Key, s.Title, firstChars(goalOf(s.DescriptionMD), 600))
	}
	if b.Len() == 0 {
		return "(none)\n"
	}
	return b.String()
}

// goalOf is a ticket's Goal section, or its opening when it has none.
func goalOf(md string) string {
	if i := strings.Index(md, "## Goal"); i >= 0 {
		rest := md[i:]
		if j := strings.Index(rest[len("## Goal"):], "\n## "); j >= 0 {
			return strings.TrimSpace(rest[:len("## Goal")+j])
		}
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(md)
}

func firstChars(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// reviewFeedback words a code review's blocking findings for the builder.
func reviewFeedback(rv models.Review) string {
	var b strings.Builder
	b.WriteString("A code reviewer (" + rv.Reviewer + ") read your change. Fix these before it goes to a person, " +
		"keep every check passing, then finish:\n")
	for _, f := range blocking(rv.Findings) {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		if loc != "" {
			fmt.Fprintf(&b, "\n- %s — %s", loc, f.Issue)
		} else {
			fmt.Fprintf(&b, "\n- %s", f.Issue)
		}
		if f.Fix != "" {
			fmt.Fprintf(&b, " Fix: %s", f.Fix)
		}
	}
	b.WriteString("\n\nIf you believe a finding is wrong, say why instead of changing the code.\n")
	return b.String()
}

// Revise sends feedback to the builder's own session in the ticket's kept
// worktree, carries on through the goal loop until the checks pass again,
// and commits what changed.
func (o *Orchestrator) Revise(ctx context.Context, ref, sessionID, feedback string) (RunResult, error) {
	scoped, issue, err := o.forTicket(ctx, ref)
	if err != nil {
		return RunResult{}, err
	}
	o = scoped
	cfg := o.Cfg.withDefaults()
	wtPath, err := o.FindWorktree(issue.Key)
	if err != nil {
		return RunResult{}, err
	}
	stored, err := o.Raenil.Criteria(ctx, issue.ID)
	if err != nil {
		return RunResult{}, err
	}
	criteria, err := ParseCriteria(stored)
	if err != nil {
		return RunResult{}, err
	}
	runDir, err := NewRunDir(cfg.RunRoot, issue.Key, 80)
	if err != nil {
		return RunResult{}, err
	}
	branch := branchForWorktree(wtPath)
	req := RunRequest{Prompt: feedback, Cwd: wtPath, Model: cfg.Model, Timeout: cfg.Timeout,
		LogPath: runDir.File("worker.log"), SessionID: sessionID, MCP: o.MCP, Repo: cfg.Repo}
	o.logf("revising %s from the code review", issue.Key)
	res, runErr := o.Runner.Run(ctx, req)
	res, runErr = o.goalLoop(ctx, o.Runner, req, res, runErr, o.goalCheck(issue.ID, wtPath, branch, runDir, criteria), issue.ID)
	if runErr != nil {
		return res, runErr
	}
	if sha, err := Commit(ctx, wtPath, fmt.Sprintf("%s: review fixes", issue.Key)); err == nil {
		_ = o.Raenil.LinkCommit(ctx, issue.ID, sha, issue.Title+" (review fixes)")
		o.logf("review fixes %s", sha[:min(8, len(sha))])
	}
	return res, nil
}

// ReviewAndRevise has a reviewer read a ticket's passed change and, while it
// finds something blocking, sends the findings to the builder's session —
// at most twice — before the change goes to the user. It returns the last
// review. A reviewer that fails is logged and skipped: the checks already
// passed, and the user still reviews.
func (o *Orchestrator) ReviewAndRevise(ctx context.Context, ref string, v Verdict, builder string, reviewer Runner, reviewerName string) (models.Review, error) {
	scoped, issue, err := o.forTicket(ctx, ref)
	if err != nil {
		return models.Review{}, err
	}
	o = scoped
	stored, err := o.Raenil.Criteria(ctx, issue.ID)
	if err != nil {
		return models.Review{}, err
	}
	criteria, err := ParseCriteria(stored)
	if err != nil {
		return models.Review{}, err
	}
	wtPath, err := o.FindWorktree(issue.Key)
	if err != nil {
		return models.Review{}, err
	}
	session := v.SessionID
	var last models.Review
	for round := 1; round <= 3; round++ {
		diff, err := o.ReviewDiff(ctx, issue.Key)
		if err != nil {
			return last, err
		}
		logPath := ""
		if rd, err := NewRunDir(o.Cfg.withDefaults().RunRoot, issue.Key, 70+round); err == nil {
			logPath = rd.File("review.log")
		}
		rv, err := o.CodeReview(ctx, issue, criteria, wtPath, diff, builder, reviewer, reviewerName, round, logPath)
		if err != nil {
			o.logf("warning: %v — skipping the code review", err)
			return last, nil
		}
		last = rv
		if rv.Verdict == "pass" || round == 3 {
			return last, nil
		}
		res, err := o.Revise(ctx, issue.Key, session, reviewFeedback(rv))
		if err != nil {
			o.logf("warning: revising from the review failed: %v", err)
			return last, nil
		}
		if res.SessionID != "" {
			session = res.SessionID
		}
	}
	return last, nil
}

// TicketFindings words a ticket review's findings as a comment.
func TicketFindings(rv models.Review) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Ticket review (%s): needs changes before it is built.**\n\n%s\n", rv.Reviewer, rv.Summary)
	for _, f := range rv.Findings {
		mark := "Minor"
		if f.Severity == "blocking" {
			mark = "Blocking"
		}
		fmt.Fprintf(&b, "\n- **%s:** %s", mark, f.Issue)
		if f.Fix != "" {
			fmt.Fprintf(&b, " — %s", f.Fix)
		}
	}
	b.WriteString("\n\nFix the ticket and move it back to Ready, or run it anyway from the ticket page.\n")
	return b.String()
}

// RunnerAvailable reports whether a runner can run here right now, for the
// runners that can tell; the rest are assumed to.
func RunnerAvailable(ctx context.Context, r Runner) error {
	type available interface {
		Available(ctx context.Context) error
	}
	if a, ok := r.(available); ok {
		return a.Available(ctx)
	}
	if oc, ok := r.(*OpenCodeRunner); ok {
		_, err := oc.Health(ctx)
		return err
	}
	return nil
}
