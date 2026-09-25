package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"raenil/internal/models"
)

// A chat turn is one reply from an agent in a ticket's conversation, the way
// Paperclip's agents talk on a task: read the thread, look at the code if it
// helps, answer — or ask structured questions — and stop. It never edits
// files: that is what a Run is for.

// ChatResult is what a turn reports back to the job.
type ChatResult struct {
	Replied   bool   `json:"replied"`
	Resumed   bool   `json:"resumed"`
	Fresh     string `json:"fresh,omitempty"` // why an existing session was not resumed
	SessionID string `json:"sessionId,omitempty"`
	RunID     string `json:"runId,omitempty"`
}

// chatInput is the job's input for a turn.
type chatInput struct {
	Reason        string `json:"reason"` // message | response
	InteractionID string `json:"interactionId"`
}

func (h *Host) chat(ctx context.Context, c *RaenilClient, job ClaimedJob) (any, error) {
	a := job.Agent
	if a == nil || job.IssueKey == "" {
		return nil, errors.New("a chat turn needs an agent and a ticket")
	}
	runner, err := h.runnerFor(*a)
	if err != nil {
		return nil, err
	}
	var in chatInput
	_ = json.Unmarshal(job.Input, &in)

	o := &Orchestrator{Raenil: c, Repos: h.Repos, Cfg: Config{Repo: h.Repo}, Log: h.Logf}
	scoped, issue, err := o.forTicket(ctx, job.IssueKey)
	if err != nil {
		return nil, err
	}
	cwd, err := filepath.Abs(scoped.Cfg.Repo)
	if err != nil {
		return nil, err
	}
	comments, err := c.Comments(ctx, issue.ID)
	if err != nil {
		return nil, fmt.Errorf("read the thread: %w", err)
	}
	interactions, _ := c.Interactions(ctx, issue.ID)
	criteria, _ := c.Criteria(ctx, issue.ID)

	// Resume where the conversation left off, when this runner can and the
	// session is still the right one; otherwise start from a full brief, which
	// carries the whole thread, so nothing said is lost.
	res := ChatResult{}
	fp := sessionFingerprint(*a, runner, AgentModel(*a), cwd)
	var session models.AgentSession
	if resumable(runner) {
		if ss, ok, err := c.AgentSession(ctx, a.ID, issue.Key); err == nil && ok {
			if why := staleSession(ss, cwd, fp, time.Now()); why != "" {
				h.logf("%s on %s: fresh session (%s)", a.Name, issue.Key, why)
				res.Fresh = why
			} else {
				session = ss
			}
		}
	}
	prompt := chatBrief(*a, issue, criteria, comments, interactions, in)
	if session.SessionID != "" {
		prompt = chatFollowUp(*a, comments, interactions, in)
		res.Resumed = true
	}

	req := RunRequest{
		Prompt:        prompt,
		Cwd:           cwd,
		Model:         AgentModel(*a),
		Timeout:       15 * time.Minute,
		ReadOnlyTools: true,
		SessionID:     session.SessionID,
	}
	if h.MCPURL != "" {
		req.MCP = &MCPServer{Name: "raenil", URL: h.MCPURL, Token: c.Token, AgentID: a.ID, Workspace: c.Workspace, Tools: ChatMCPTools}
	}

	logFile, err := os.CreateTemp("", "raenil-chat-*.log")
	if err != nil {
		return nil, err
	}
	logFile.Close()
	defer os.Remove(logFile.Name())
	req.LogPath = logFile.Name()

	rec, recErr := c.StartRun(ctx, RunStartReq{IssueID: issue.ID, AgentID: a.ID, Kind: "chat",
		Runner: runner.Name(), Model: effectiveModel(runner, req.Model), Attempt: 1, Host: h.Name})
	if recErr != nil {
		h.logf("could not record the chat run: %v", recErr)
	}
	res.RunID = rec.ID

	stopStream := streamLog(ctx, c, rec.ID, req.LogPath, runner.Name())
	out, runErr := runner.Run(ctx, req)
	// A session that cannot be resumed (expired, moved) is retried cold, once.
	if res.Resumed && (runErr != nil || out.Exit != 0) && out.Answer == "" {
		h.logf("resume failed (%v %s); starting a fresh session", runErr, out.AgentError)
		req.SessionID, req.Prompt, res.Resumed = "", chatBrief(*a, issue, criteria, comments, interactions, in), false
		// Its own log, so the live transcript follows the retry from its start.
		stopStream()
		if retry, err := os.CreateTemp("", "raenil-chat-*.log"); err == nil {
			retry.Close()
			defer os.Remove(retry.Name())
			req.LogPath = retry.Name()
		}
		stopStream = streamLog(ctx, c, rec.ID, req.LogPath, runner.Name())
		out, runErr = runner.Run(ctx, req)
	}
	stopStream()
	if rec.ID != "" {
		o.finishRun(context.WithoutCancel(ctx), rec.ID, runner.Name(), req.LogPath, out, runErr, "", "")
	}
	if runErr != nil {
		return res, runErr
	}
	if out.Exit != 0 || out.Aborted {
		return res, fmt.Errorf("%s did not finish its turn: %s", a.Name, firstNonEmpty(out.AgentError, fmt.Sprintf("exit %d", out.Exit)))
	}

	if answer := strings.TrimSpace(out.Answer); answer != "" {
		if err := c.CommentAs(ctx, issue.ID, answer, a.ID); err != nil {
			return res, fmt.Errorf("post the reply: %w", err)
		}
		res.Replied = true
	}
	if out.SessionID != "" {
		res.SessionID = out.SessionID
		if err := c.SaveAgentSession(ctx, a.ID, issue.Key, out.SessionID, cwd, fp); err != nil {
			h.logf("could not save the session: %v", err)
		}
	}
	return res, nil
}

// A session is retired after this many turns or this long, as Paperclip
// rotates its own: a long session costs more per turn than a fresh brief.
const (
	maxSessionTurns = 30
	maxSessionAge   = 72 * time.Hour
)

// sessionFingerprint names the setup a session began in. Resuming into a
// changed setup — another repository, harness, model or set of instructions —
// would carry the old context into the wrong place.
func sessionFingerprint(a models.Agent, r Runner, model, cwd string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{r.Name(), model, a.Effort, cwd, a.InstructionsMD}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// staleSession says why a session must not be resumed, or "" when it can be.
func staleSession(ss models.AgentSession, cwd, fp string, now time.Time) string {
	switch {
	case ss.SessionID == "":
		return "none yet"
	case ss.Cwd != cwd:
		return "the repository moved"
	case ss.Fingerprint != fp:
		return "the agent's setup changed"
	case ss.Turns >= maxSessionTurns:
		return fmt.Sprintf("%d turns", ss.Turns)
	case now.Sub(ss.CreatedAt) >= maxSessionAge:
		return "older than 72h"
	}
	return ""
}

// resumable reports whether a runner can continue a session by id.
func resumable(r Runner) bool {
	switch r.(type) {
	case *ClaudeRunner, *OpenCodeRunner:
		return true
	}
	return false
}

// chatBrief is the opening of a conversation: who the agent is, how Raenil
// works, the ticket, and the thread so far.
func chatBrief(a models.Agent, is models.Issue, criteria []models.Criterion, comments []models.Comment,
	interactions []models.Interaction, in chatInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s", a.Name)
	if a.Role != "" {
		fmt.Fprintf(&b, " (%s)", a.Role)
	}
	fmt.Fprintf(&b, ". You are talking with the user on ticket %s in Raenil, their issue tracker.\n\n", is.Key)
	if strings.TrimSpace(a.InstructionsMD) != "" {
		b.WriteString("## Your instructions\n\n" + strings.TrimSpace(a.InstructionsMD) + "\n\n")
	}
	b.WriteString(`## How this conversation works

- This is a conversation turn, not implementation. Read the code in this repository when it helps you answer, but do not change any file: implementation happens later, when the user runs the ticket.
- Your final message is posted on the ticket as your reply. Keep it short and concrete; use markdown.
- When you need decisions from the user, call the Raenil tool ask_user with a few questions, each with concrete options where you can. Then end your turn — do not guess the answers.
- When the work is understood well enough to build, you can propose splitting it into tickets with propose_tickets: each ticket with a title, a description, and done-when criteria a program can check (a command that must exit 0, or a path policy). The user approves the split.

`)
	fmt.Fprintf(&b, "## Ticket %s: %s\n\n", is.Key, is.Title)
	if d := strings.TrimSpace(is.DescriptionMD); d != "" {
		b.WriteString(d + "\n\n")
	}
	if len(criteria) > 0 {
		b.WriteString("### Done when\n\n")
		for _, c := range criteria {
			mark := " "
			if c.Done {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", mark, c.Body)
		}
		b.WriteString("\n")
	}
	if thread := renderThread(a, comments, interactions); thread != "" {
		b.WriteString("## The conversation so far\n\n" + thread + "\n")
	}
	b.WriteString("## Your turn\n\n" + turnAsk(in))
	return b.String()
}

// chatFollowUp is a resumed turn: only what happened since the agent last spoke.
func chatFollowUp(a models.Agent, comments []models.Comment, interactions []models.Interaction, in chatInput) string {
	since := lastSpoke(a, comments)
	var news []models.Comment
	for _, c := range comments {
		if c.CreatedAt.After(since) && (c.AgentID == nil || *c.AgentID != a.ID) {
			news = append(news, c)
		}
	}
	var b strings.Builder
	if len(news) > 0 {
		b.WriteString("New on the ticket since your last reply:\n\n")
		for _, c := range news {
			b.WriteString(renderComment(a, c))
		}
		b.WriteString("\n")
	}
	if in.Reason == "response" {
		for _, it := range interactions {
			if it.ID == in.InteractionID {
				b.WriteString(renderResponse(it) + "\n")
			}
		}
	}
	b.WriteString(turnAsk(in))
	return b.String()
}

func turnAsk(in chatInput) string {
	switch in.Reason {
	case "response":
		return "The user answered your questions (above). Continue from their answers.\n"
	case "start":
		return "The user has started this task. Read the ticket and its done-when, and look at the code it " +
			"concerns. Then do one of these, whichever fits:\n" +
			"- If decisions only the user can make are open, ask them with ask_user.\n" +
			"- If the work is too big for one ticket, propose the split with propose_tickets.\n" +
			"- If it is clear and its done-when can be checked by a program, say it is ready to run, " +
			"and in a few lines how you would do it.\n" +
			"- If it is clear but the done-when is missing or only manual, say which checks it should have.\n"
	}
	return "Reply to the user's latest message.\n"
}

func lastSpoke(a models.Agent, comments []models.Comment) time.Time {
	var t time.Time
	for _, c := range comments {
		if c.AgentID != nil && *c.AgentID == a.ID && c.CreatedAt.After(t) {
			t = c.CreatedAt
		}
	}
	return t
}

// renderThread interleaves comments and interactions in time order.
func renderThread(a models.Agent, comments []models.Comment, interactions []models.Interaction) string {
	type entry struct {
		at   time.Time
		text string
	}
	var es []entry
	for _, c := range comments {
		es = append(es, entry{c.CreatedAt, renderComment(a, c)})
	}
	for _, it := range interactions {
		es = append(es, entry{it.CreatedAt, renderAsk(it)})
		if it.ResolvedAt != nil {
			es = append(es, entry{*it.ResolvedAt, renderResponse(it)})
		}
	}
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && es[j].at.Before(es[j-1].at); j-- {
			es[j], es[j-1] = es[j-1], es[j]
		}
	}
	var b strings.Builder
	for _, e := range es {
		b.WriteString(e.text)
	}
	return b.String()
}

func renderComment(a models.Agent, c models.Comment) string {
	who := "User"
	switch {
	case c.AgentID != nil && *c.AgentID == a.ID:
		who = "You"
	case c.AgentID != nil || c.Actor == "ai":
		who = "Another agent"
	}
	return fmt.Sprintf("**%s:** %s\n\n", who, strings.TrimSpace(c.BodyMD))
}

func renderAsk(it models.Interaction) string {
	if it.Kind != "questions" {
		return "_A proposal was made (" + it.Status + ")._\n\n"
	}
	var p struct {
		Questions []models.Question `json:"questions"`
	}
	_ = json.Unmarshal(it.Payload, &p)
	var b strings.Builder
	b.WriteString("_Questions asked:_\n")
	for _, q := range p.Questions {
		fmt.Fprintf(&b, "- %s", q.Text)
		if len(q.Options) > 0 {
			fmt.Fprintf(&b, " (options: %s)", strings.Join(q.Options, " / "))
		}
		b.WriteString("\n")
	}
	return b.String() + "\n"
}

func renderResponse(it models.Interaction) string {
	switch it.Status {
	case "answered":
		var q struct {
			Questions []models.Question `json:"questions"`
		}
		var r struct {
			Answers []models.Answer `json:"answers"`
		}
		_ = json.Unmarshal(it.Payload, &q)
		_ = json.Unmarshal(it.Response, &r)
		text := map[string]string{}
		for _, x := range q.Questions {
			text[x.ID] = x.Text
		}
		var b strings.Builder
		b.WriteString("**User answered:**\n")
		for _, ans := range r.Answers {
			parts := append([]string{}, ans.Choices...)
			if o := strings.TrimSpace(ans.Other); o != "" {
				parts = append(parts, o)
			}
			fmt.Fprintf(&b, "- %s → %s\n", text[ans.QuestionID], strings.Join(parts, "; "))
		}
		return b.String() + "\n"
	case "approved", "rejected":
		var r struct {
			Note string `json:"note"`
		}
		_ = json.Unmarshal(it.Response, &r)
		s := "**User " + it.Status + " the proposal.**"
		if r.Note != "" {
			s += " " + r.Note
		}
		return s + "\n\n"
	}
	return ""
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
