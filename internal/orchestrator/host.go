package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"raenil/internal/models"
)

// Host is a runner host: the machine agents actually run on. Raenil queues
// work; the host claims what it can run, does it, and reports back. It also
// tells Raenil, every heartbeat, which harnesses it has proved ready — which is
// what the Connectors page shows.
type Host struct {
	Name    string
	Version string
	// Clients holds one client per workspace this host serves.
	Clients []*RaenilClient
	Runners RunnerSet
	// Probe checks one harness. Empty means probeHarness.
	Probe func(ctx context.Context, harness string, r Runner) models.HarnessStatus
	// RunTicket works a ticket for an agent. Nil means run_ticket jobs fail
	// with a clear message rather than being claimed and dropped.
	RunTicket func(ctx context.Context, c *RaenilClient, job ClaimedJob) (any, error)
	// Review verifies (finish=false) or finishes a handed-back ticket, as
	// `orchestrator verify` / `finish` do. Nil refuses those jobs.
	Review func(ctx context.Context, c *RaenilClient, job ClaimedJob, finish bool) (any, error)
	// RunnerFor builds the runner an agent asked for, with its settings.
	// Empty means the host's runner for the agent's harness.
	RunnerFor func(models.Agent) (Runner, error)

	// Repos and Repo say where a ticket's code is, as for `work`: a chat turn
	// reads the same repository a run would change.
	Repos RepoSet
	Repo  string
	// MCPURL is Raenil's MCP endpoint, handed to an agent in a chat turn so it
	// can ask questions and propose tickets. Empty leaves it without tools.
	MCPURL string

	Poll      time.Duration // how often to ask for work (default 3s)
	Heartbeat time.Duration // how often to report health (default 30s)
	Logf      func(format string, args ...any)

	mu     sync.Mutex
	status []models.HarnessStatus
	busy   bool // a job is running
	jobs   sync.WaitGroup
}

func (h *Host) logf(format string, args ...any) {
	if h.Logf != nil {
		h.Logf(format, args...)
	}
}

// Run serves until ctx ends. Jobs run one at a time — a Mac running two agents
// in one repo at once is how worktrees and tokens collide — but beside the
// heartbeat, so a long run does not make the host look gone.
func (h *Host) Run(ctx context.Context) error {
	if len(h.Clients) == 0 {
		return errors.New("host serves no workspace")
	}
	if h.Poll <= 0 {
		h.Poll = 3 * time.Second
	}
	if h.Heartbeat <= 0 {
		h.Heartbeat = 30 * time.Second
	}
	h.beat(ctx, true)
	beat := time.NewTicker(h.Heartbeat)
	defer beat.Stop()
	poll := time.NewTicker(h.Poll)
	defer poll.Stop()
	defer h.jobs.Wait()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-beat.C:
			h.beat(ctx, false)
		case <-poll.C:
			h.pollOnce(ctx)
		}
	}
}

// beat re-checks every harness and reports the result to every workspace.
// The first beat after start-up also tells Raenil to release what this host
// had claimed before it went down.
func (h *Host) beat(ctx context.Context, started bool) {
	probe := h.Probe
	if probe == nil {
		probe = probeHarness
	}
	var status []models.HarnessStatus
	for _, name := range models.Harnesses {
		status = append(status, probe(ctx, name, h.Runners[name]))
	}
	h.mu.Lock()
	h.status = status
	h.mu.Unlock()
	for _, c := range h.Clients {
		if err := c.Heartbeat(ctx, h.Name, h.Version, status, started); err != nil {
			h.logf("heartbeat to %s: %v", c.Workspace, err)
		}
	}
}

// ready lists the harnesses this host can take work for right now.
func (h *Host) ready() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, s := range h.status {
		if s.Ready {
			out = append(out, s.Harness)
		}
	}
	return out
}

// pollOnce claims one job when the host is idle and runs it in the
// background, so the heartbeat keeps going while it works.
func (h *Host) pollOnce(ctx context.Context) {
	h.mu.Lock()
	if h.busy {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()
	harnesses := h.ready()
	if len(harnesses) == 0 {
		return
	}
	for _, c := range h.Clients {
		job, ok, err := c.ClaimJob(ctx, h.Name, harnesses)
		if err != nil {
			h.logf("claim from %s: %v", c.Workspace, err)
			continue
		}
		if !ok {
			continue
		}
		h.mu.Lock()
		h.busy = true
		h.mu.Unlock()
		h.jobs.Add(1)
		go func() {
			defer h.jobs.Done()
			defer func() {
				h.mu.Lock()
				h.busy = false
				h.mu.Unlock()
			}()
			h.handle(ctx, c, job)
		}()
		return
	}
}

func (h *Host) handle(ctx context.Context, c *RaenilClient, job ClaimedJob) {
	h.logf("job %s: %s for %s", job.ID[:8], job.Kind, agentLabel(job.Agent))
	var result any
	var err error
	switch job.Kind {
	case "test_env":
		result, err = h.testEnv(ctx, job)
	case "chat":
		result, err = h.chat(ctx, c, job)
	case "verify", "finish":
		if h.Review == nil {
			err = errors.New("this host cannot review tickets")
		} else {
			result, err = h.Review(ctx, c, job, job.Kind == "finish")
		}
	case "run_ticket":
		if h.RunTicket == nil {
			err = errors.New("this host cannot run tickets")
		} else {
			result, err = h.RunTicket(ctx, c, job)
		}
	default:
		err = fmt.Errorf("unknown job kind %q", job.Kind)
	}
	status, errText := "succeeded", ""
	if err != nil {
		status, errText = "failed", err.Error()
		h.logf("job %s failed: %v", job.ID[:8], err)
	}
	if ferr := c.FinishJob(context.WithoutCancel(ctx), job.ID, h.Name, status, result, errText); ferr != nil {
		h.logf("job %s: could not report the result: %v", job.ID[:8], ferr)
	}
}

// TestResult is what an environment test proved.
type TestResult struct {
	Harness    string `json:"harness"`
	Model      string `json:"model"`
	Answer     string `json:"answer"`
	DurationMS int64  `json:"durationMs"`
}

// testEnv proves the agent's harness answers with its model, end to end — the
// check Paperclip's "Test environment" button runs.
func (h *Host) testEnv(ctx context.Context, job ClaimedJob) (any, error) {
	if job.Agent == nil {
		return nil, errors.New("the job names no agent")
	}
	r, ok := h.Runners[job.Agent.Harness]
	if !ok {
		return nil, fmt.Errorf("%s is not set up on %s", job.Agent.Harness, h.Name)
	}
	asker, ok := r.(Asker)
	if !ok {
		return nil, fmt.Errorf("%s cannot answer a test prompt", job.Agent.Harness)
	}
	model := AgentModel(*job.Agent)
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	answer, _, err := asker.Ask(ctx, model, "Reply with exactly: hello")
	res := TestResult{Harness: job.Agent.Harness, Model: effectiveModel(r, model), Answer: trunc(answer, 200),
		DurationMS: time.Since(start).Milliseconds()}
	if err != nil {
		return res, err
	}
	if !strings.Contains(strings.ToLower(answer), "hello") {
		return res, fmt.Errorf("the model answered, but not as asked: %q", trunc(answer, 120))
	}
	return res, nil
}

// AgentModel turns an agent's model into the provider/model string a runner
// reads. Claude and Codex models are addressed to their own runner; OpenCode
// models already carry their provider.
func AgentModel(a models.Agent) string {
	m := strings.TrimSpace(a.Model)
	switch a.Harness {
	case "claude", "codex":
		if m == "" {
			m = "default"
		}
		if !strings.HasPrefix(m, a.Harness+"/") {
			m = a.Harness + "/" + m
		}
	}
	return m
}

func agentLabel(a *models.Agent) string {
	if a == nil {
		return "no agent"
	}
	return a.Name + " (" + a.Harness + ")"
}

// probeHarness checks one harness without spending a model call: is it
// installed, signed in, on the kind of plan it should be.
func probeHarness(ctx context.Context, harness string, r Runner) models.HarnessStatus {
	st := models.HarnessStatus{Harness: harness, CheckedAt: time.Now()}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	switch rr := r.(type) {
	case *ClaudeRunner:
		st.Models = []string{"default", "sonnet", "opus", "haiku"}
		err := rr.Available(ctx)
		st.Installed = err == nil || !strings.Contains(err.Error(), "not on PATH")
		st.Ready = err == nil
		st.Auth = "Claude subscription · Mac login"
		if rr.OAuthToken != "" {
			st.Auth = "Claude subscription · Raenil connection"
		}
		if err != nil {
			st.Detail = err.Error()
			st.Fix = "orchestrator connect claude"
		}
	case *CodexRunner:
		st.Models = []string{"default"}
		err := rr.Available(ctx)
		st.Installed = err == nil || !strings.Contains(err.Error(), "not on PATH")
		st.Ready = err == nil
		st.Auth = "ChatGPT · Mac login"
		if rr.Home != "" {
			st.Auth = "ChatGPT · Raenil connection"
		}
		if err != nil {
			st.Detail = err.Error()
			st.Fix = "orchestrator connect codex"
		}
	case *OpenCodeRunner:
		st.Auth = "API key (OpenCode Go)"
		v, err := rr.Health(ctx)
		st.Installed = err == nil
		st.Ready = err == nil
		if err != nil {
			st.Detail = err.Error()
			st.Fix = "opencode serve, and set OPENCODE_URL for the host"
		} else {
			st.Detail = "server " + v
		}
	case nil:
		st.Detail = "not configured on this host"
		switch harness {
		case "claude":
			st.Fix = "install Claude Code, then: claude auth login"
		case "codex":
			st.Fix = "install Codex, then: codex login"
		case "opencode":
			st.Fix = "opencode serve, and set OPENCODE_URL for the host"
		}
	default:
		st.Detail = "unknown runner"
	}
	return st
}

// runnerFor is the runner for an agent: RunnerFor when set, else the host's.
func (h *Host) runnerFor(a models.Agent) (Runner, error) {
	if h.RunnerFor != nil {
		return h.RunnerFor(a)
	}
	if r, ok := h.Runners[a.Harness]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("%s is not set up on %s", a.Harness, h.Name)
}
