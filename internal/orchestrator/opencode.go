package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// OpenCodeRunner drives a headless `opencode serve` over its HTTP API.
//
// The HTTP path is used in preference to the `opencode run` CLI on purpose: the
// CLI hangs forever — silently, with no output on either stream — when stdin is
// an open pipe, which is exactly the shape a daemon would invoke it with.
type OpenCodeRunner struct {
	// BaseURL is the running server, e.g. http://127.0.0.1:4096
	BaseURL string
	// HTTP is the client used for every call except the blocking wait.
	HTTP *http.Client
	// Agent optionally selects a configured opencode agent.
	Agent string
	// Permission decides each tool-call permission request. Nil means
	// ApproveOncePolicy.
	Permission PermissionPolicy
	// PollInterval controls how often pending permissions and questions are
	// drained while an attempt runs.
	PollInterval time.Duration

	ready readyCache
}

// Ready proves this model can actually produce output, before a ticket is
// claimed for it.
//
// Health and catalog membership are checked first because they give a clearer
// message, but neither is sufficient: a provider can be declared in config,
// list its models, and still fail to load because its npm package was never
// installed. Only a completion settles it.
func (r *OpenCodeRunner) Ready(ctx context.Context, model string) error {
	provider, m := splitModel(model)
	if m == "" {
		return fmt.Errorf("no model given")
	}
	if provider == "" {
		return fmt.Errorf("model %q needs a provider prefix, e.g. opencode-go/glm-5.3-flash", model)
	}
	return r.ready.once(model, func() error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if _, err := r.Health(ctx); err != nil {
			return fmt.Errorf("server unreachable: %w", err)
		}
		if err := r.modelInCatalog(ctx, provider, m); err != nil {
			return err
		}
		dir, err := os.MkdirTemp("", "raenil-ready-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		answer, _, err := r.askIn(ctx, model, "Reply with exactly: ok", dir, true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(answer) == "" {
			return fmt.Errorf("model produced no output")
		}
		return nil
	})
}

// modelInCatalog reports whether the server actually offers this model.
func (r *OpenCodeRunner) modelInCatalog(ctx context.Context, provider, model string) error {
	var cat struct {
		Providers []struct {
			ID     string                     `json:"id"`
			Models map[string]json.RawMessage `json:"models"`
		} `json:"providers"`
	}
	if err := r.do(ctx, r.client(), http.MethodGet, "/config/providers", nil, nil, &cat); err != nil {
		return fmt.Errorf("could not read the provider catalog: %w", err)
	}
	for _, p := range cat.Providers {
		if p.ID != provider {
			continue
		}
		if _, ok := p.Models[model]; ok {
			return nil
		}
		have := make([]string, 0, len(p.Models))
		for k := range p.Models {
			have = append(have, k)
		}
		sort.Strings(have)
		return fmt.Errorf("provider %q does not offer %q (offers: %s)", provider, model, strings.Join(have, ", "))
	}
	return fmt.Errorf("provider %q is not configured", provider)
}

// PermissionDecision is the reply to a pending permission request.
type PermissionDecision string

const (
	// PermissionOnce allows this single call.
	PermissionOnce PermissionDecision = "once"
	// PermissionAlways allows this call and remembers the pattern.
	PermissionAlways PermissionDecision = "always"
	// PermissionReject refuses the call. The agent is told why.
	PermissionReject PermissionDecision = "reject"
)

// PermissionRequest is a pending tool-call approval.
type PermissionRequest struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"sessionID"`
	Permission string         `json:"permission"`
	Patterns   []string       `json:"patterns,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// PermissionPolicy decides one request. Returning PermissionReject with a reason
// is the prevention layer: the tool call never happens, rather than being caught
// afterwards by the diff gate.
type PermissionPolicy func(PermissionRequest) (PermissionDecision, string)

// ApproveOncePolicy allows every tool call. Safe only because the agent is
// confined to a disposable worktree and the policy gates still run on the diff.
func ApproveOncePolicy(PermissionRequest) (PermissionDecision, string) {
	return PermissionOnce, ""
}

// QuestionRequest is a pending question the agent asked.
type QuestionRequest struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	Text      string `json:"text,omitempty"`
}

// ocMessage is one entry from GET /session/{id}/message. The server nests the
// metadata under "info" and the content under "parts".
type ocMessage struct {
	Info struct {
		ID   string `json:"id"`
		Role string `json:"role"`
		Time struct {
			Created   float64 `json:"created"`
			Completed float64 `json:"completed"`
		} `json:"time"`
		Agent  string          `json:"agent"`
		Finish string          `json:"finish"`
		Cost   float64         `json:"cost"`
		Error  json.RawMessage `json:"error"`
		Tokens struct {
			Input     float64 `json:"input"`
			Output    float64 `json:"output"`
			Reasoning float64 `json:"reasoning"`
		} `json:"tokens"`
	} `json:"info"`
	Parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"parts"`
}

type ocSession struct {
	ID     string  `json:"id"`
	Cost   float64 `json:"cost"`
	Tokens struct {
		Input     float64 `json:"input"`
		Output    float64 `json:"output"`
		Reasoning float64 `json:"reasoning"`
	} `json:"tokens"`
}

// Name implements Runner.
func (r *OpenCodeRunner) Name() string { return "opencode" }

func (r *OpenCodeRunner) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (r *OpenCodeRunner) pollInterval() time.Duration {
	if r.PollInterval > 0 {
		return r.PollInterval
	}
	return 500 * time.Millisecond
}

// do performs one JSON request. out may be nil when the response is discarded.
func (r *OpenCodeRunner) do(ctx context.Context, c *http.Client, method, path string, q url.Values, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	u := r.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("opencode %s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(b))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Health reports whether the server is up, for a daemon's liveness check.
func (r *OpenCodeRunner) Health(ctx context.Context) (string, error) {
	var h struct {
		Healthy bool   `json:"healthy"`
		Version string `json:"version"`
	}
	if err := r.do(ctx, r.client(), http.MethodGet, "/global/health", nil, nil, &h); err != nil {
		return "", err
	}
	if !h.Healthy {
		return h.Version, fmt.Errorf("opencode reports unhealthy")
	}
	return h.Version, nil
}

// Run implements Runner.
func (r *OpenCodeRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if req.Terminal && !req.DisableTools && !req.ReadOnlyTools {
		return r.runTerminal(ctx, req)
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultRunTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	// OpenCode runs on an API key (OpenCode Go included): metered spend.
	res := RunResult{Billing: "api"}
	dirQ := url.Values{"directory": {req.Cwd}}

	// 1. Session. Continuing an existing one keeps a failed attempt in context,
	// which is what makes a repair cheaper than a cold retry.
	sessionID := req.SessionID
	if sessionID == "" {
		var s ocSession
		body := map[string]any{"title": "orchestrator attempt"}
		if !req.DisableTools && !req.ReadOnlyTools {
			body["permission"] = openCodePermissions(req.Repo)
		}
		if err := r.do(ctx, r.client(), http.MethodPost, "/session", dirQ, body, &s); err != nil {
			return res, fmt.Errorf("create session: %w", err)
		}
		sessionID = s.ID
	}
	res.SessionID = sessionID

	// Raenil's tools for this run: registered on the server for its duration,
	// then disconnected, so the next run — maybe another agent's, maybe a judge
	// that should have none — never inherits them. The registration belongs to
	// the directory, and the host runs jobs in parallel: runs in one directory
	// (conversation turns in the same repo) take turns so none sees another
	// agent's tools or has them disconnected under it. Work runs each have
	// their own worktree and run side by side.
	if req.MCP != nil && !req.DisableTools {
		defer lockDir(req.Cwd)()
		if err := r.addMCP(ctx, dirQ, *req.MCP); err != nil {
			return res, fmt.Errorf("give the run Raenil's tools: %w", err)
		}
		defer r.do(context.WithoutCancel(ctx), r.client(), http.MethodPost,
			"/mcp/"+req.MCP.Name+"/disconnect", dirQ, nil, nil)
	}

	// 2. Dispatch without blocking, so permissions can be serviced while it runs.
	prompt := map[string]any{
		"parts": []map[string]any{{"type": "text", "text": req.Prompt}},
	}
	if provider, model := splitModel(req.Model); model != "" {
		if provider == "" {
			return res, fmt.Errorf("model %q needs a provider prefix, e.g. opencode-go/glm-5.3-flash", req.Model)
		}
		prompt["model"] = map[string]string{"providerID": provider, "modelID": model}
	}
	if r.Agent != "" {
		prompt["agent"] = r.Agent
	}
	switch {
	case req.DisableTools:
		if tools, err := r.toolPolicy(ctx, nil); err == nil && len(tools) > 0 {
			prompt["tools"] = tools
		}
	case req.ReadOnlyTools:
		if tools, err := r.toolPolicy(ctx, readOnlyToolIDs); err == nil && len(tools) > 0 {
			prompt["tools"] = tools
		}
	}
	if err := r.do(ctx, r.client(), http.MethodPost,
		"/session/"+sessionID+"/prompt_async", dirQ, prompt, nil); err != nil {
		return res, fmt.Errorf("dispatch prompt: %w", err)
	}

	// 3. Poll for completion, servicing permissions and questions as they appear.
	//
	// The obvious call here would be POST /api/session/{id}/wait, but that
	// endpoint is published in the OpenAPI spec and NOT implemented by the
	// server: it answers 503 "Session wait is not available yet" (verified
	// against 1.18.31). The reliable signal is the last assistant message
	// carrying a non-zero time.completed.
	ticker := time.NewTicker(r.pollInterval())
	defer ticker.Stop()

	var agentErr json.RawMessage
loop:
	for {
		select {
		case <-ctx.Done():
			res.Aborted = true
			// Abort needs a live context of its own — ctx is already done.
			abortCtx, abortCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			_ = r.do(abortCtx, r.client(), http.MethodPost, "/session/"+sessionID+"/abort", dirQ, nil, nil)
			abortCancel()
			break loop
		case <-ticker.C:
			r.servicePending(ctx, sessionID, &res)
			done, cost, tokens, aerr, err := r.pollCompletion(ctx, sessionID, req.Cwd)
			if err != nil {
				continue // a transient read is not a failed attempt
			}
			if done {
				// Drain once more: a permission can land in the same instant the
				// attempt finishes.
				r.servicePending(ctx, sessionID, &res)
				res.CostUSD, res.Tokens, agentErr = cost, tokens, aerr
				break loop
			}
		}
	}

	res.Duration = time.Since(start)

	// 4. If the attempt was aborted we never read the messages; fall back to the
	// session's own totals, on a context that outlives the abort.
	tailCtx, tailCancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer tailCancel()
	if res.Tokens == 0 {
		var s ocSession
		if err := r.do(tailCtx, r.client(), http.MethodGet, "/session/"+sessionID, dirQ, nil, &s); err == nil {
			res.CostUSD = s.Cost
			res.Tokens = int(s.Tokens.Input + s.Tokens.Output + s.Tokens.Reasoning)
		}
	}

	// 5. Transcript to disk. This file exists for humans and scripts; it must
	// never be piped back into a model's context.
	if req.LogPath != "" {
		r.dumpTranscript(tailCtx, sessionID, req.Cwd, req.LogPath)
	}

	switch {
	case res.Aborted:
		res.Exit = 124 // conventional timeout exit code
	case len(agentErr) > 0 && string(agentErr) != "null":
		// The agent finished, but reported an error. That is a failed attempt,
		// not a broken orchestrator, so it is a result rather than an error.
		res.Exit = 1
		res.AgentError = summariseAgentError(agentErr)
	default:
		res.Exit = 0
		// The final reply, for a host that posts it — a conversation turn.
		res.Answer, _ = r.finalAnswer(tailCtx, sessionID, req.Cwd)
	}
	return res, nil
}

// finalAnswer reads the last assistant text of a finished session. A
// message's completed timestamp can land fractionally before its text parts
// are readable, so a single fetch intermittently sees an answer with no
// content: poll briefly rather than report silence the model did not mean.
func (r *OpenCodeRunner) finalAnswer(ctx context.Context, sessionID, dir string) (string, error) {
	q := url.Values{"directory": {dir}}
	deadline := time.Now().Add(15 * time.Second)
	for {
		var msgs []ocMessage
		if err := r.do(ctx, r.client(), http.MethodGet, "/session/"+sessionID+"/message", q, nil, &msgs); err != nil {
			return "", err
		}
		if text := lastAssistantText(msgs); text != "" {
			return text, nil
		}
		if time.Now().After(deadline) {
			return "", nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// pollCompletion reports whether the agent has finished its turn, along with the
// cost and tokens accumulated across its assistant messages.
func (r *OpenCodeRunner) pollCompletion(ctx context.Context, sessionID, cwd string) (
	done bool, cost float64, tokens int, agentErr json.RawMessage, err error) {

	var msgs []ocMessage
	q := url.Values{"directory": {cwd}}
	if err = r.do(ctx, r.client(), http.MethodGet, "/session/"+sessionID+"/message", q, nil, &msgs); err != nil {
		return false, 0, 0, nil, err
	}

	var lastAssistant *ocMessage
	for i := range msgs {
		m := &msgs[i]
		if m.Info.Role != "assistant" {
			continue
		}
		cost += m.Info.Cost
		tokens += int(m.Info.Tokens.Input + m.Info.Tokens.Output + m.Info.Tokens.Reasoning)
		lastAssistant = m
	}
	if lastAssistant == nil || lastAssistant.Info.Time.Completed == 0 {
		return false, 0, 0, nil, nil
	}
	return true, cost, tokens, lastAssistant.Info.Error, nil
}

// servicePending answers whatever the agent is blocked on. Questions are always
// rejected: a daemon cannot answer one, and a rejected question captured here
// becomes the escalation payload instead of a silent stall.
func (r *OpenCodeRunner) servicePending(ctx context.Context, sessionID string, res *RunResult) {
	policy := r.Permission
	if policy == nil {
		policy = ApproveOncePolicy
	}

	var perms []PermissionRequest
	if err := r.do(ctx, r.client(), http.MethodGet, "/permission", nil, nil, &perms); err == nil {
		for _, p := range perms {
			if p.SessionID != "" && p.SessionID != sessionID {
				continue
			}
			decision, reason := policy(p)
			body := map[string]any{"reply": string(decision)}
			if reason != "" {
				body["message"] = reason
			}
			if err := r.do(ctx, r.client(), http.MethodPost,
				"/permission/"+p.ID+"/reply", nil, body, nil); err != nil {
				continue
			}
			if decision == PermissionReject {
				res.DeniedTools = append(res.DeniedTools, p.Permission)
			}
		}
	}

	var questions []QuestionRequest
	if err := r.do(ctx, r.client(), http.MethodGet, "/question", nil, nil, &questions); err == nil {
		for _, q := range questions {
			if q.SessionID != "" && q.SessionID != sessionID {
				continue
			}
			if q.Text != "" {
				res.Questions = append(res.Questions, q.Text)
			}
			_ = r.do(ctx, r.client(), http.MethodPost, "/question/"+q.ID+"/reject", nil, nil, nil)
		}
	}
}

// dumpTranscript writes the session's messages to disk as raw JSON.
func (r *OpenCodeRunner) dumpTranscript(ctx context.Context, sessionID, cwd, path string) {
	var msgs json.RawMessage
	q := url.Values{"directory": {cwd}}
	if err := r.do(ctx, r.client(), http.MethodGet, "/session/"+sessionID+"/message", q, nil, &msgs); err != nil {
		return
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, msgs, "", "  ") != nil {
		pretty.Write(msgs)
	}
	_ = os.WriteFile(path, pretty.Bytes(), 0o644)
}

// SetModel switches a live session's model. This is the escalation path: a
// ticket stuck on a cheap model moves to a frontier one with the failed attempt
// still in context, rather than restarting cold.
func (r *OpenCodeRunner) SetModel(ctx context.Context, sessionID, model string) error {
	provider, m := splitModel(model)
	if provider == "" {
		return fmt.Errorf("model %q needs a provider prefix", model)
	}
	return r.do(ctx, r.client(), http.MethodPost, "/api/session/"+sessionID+"/model", nil,
		map[string]string{"providerID": provider, "modelID": m}, nil)
}

// Fork branches a session so a repair attempt can start from a failed one
// without mutating the original record.
func (r *OpenCodeRunner) Fork(ctx context.Context, sessionID, cwd string) (string, error) {
	var s ocSession
	q := url.Values{"directory": {cwd}}
	if err := r.do(ctx, r.client(), http.MethodPost, "/session/"+sessionID+"/fork", q, nil, &s); err != nil {
		return "", err
	}
	return s.ID, nil
}

var _ Runner = (*OpenCodeRunner)(nil)

// readOnlyToolIDs are the tools a reviewer may use: enough to look, nothing to
// change. Anything not listed is turned off.
var readOnlyToolIDs = map[string]bool{"read": true, "glob": true, "grep": true}

// toolPolicy builds a tools map from the server's own tool list, enabling only
// the named ones. Fetched rather than hardcoded so it stays correct as opencode
// gains tools — a new write tool defaults to off rather than silently allowed.
func (r *OpenCodeRunner) toolPolicy(ctx context.Context, allow map[string]bool) (map[string]bool, error) {
	var ids []string
	if err := r.do(ctx, r.client(), http.MethodGet, "/experimental/tool/ids", nil, nil, &ids); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = allow[id]
	}
	return out, nil
}

// Ask runs a one-shot prompt and returns the assistant's text. It is the
// judgment plane's way in: no tools, no worktree, just an opinion.
func (r *OpenCodeRunner) Ask(ctx context.Context, model, prompt string) (string, float64, error) {
	dir, err := os.MkdirTemp("", "raenil-judge-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(dir)
	return r.askIn(ctx, model, prompt, dir, true)
}

// AskIn answers from inside a directory with read-only tools available.
func (r *OpenCodeRunner) AskIn(ctx context.Context, model, prompt, cwd string) (string, float64, error) {
	return r.askIn(ctx, model, prompt, cwd, false)
}

// askIn returns the answer and what it cost. The cost is returned rather than
// discarded because a judge that spends money invisibly defeats the budget
// guard: the ceiling can only stop what it can see.
func (r *OpenCodeRunner) askIn(ctx context.Context, model, prompt, dir string, noTools bool) (string, float64, error) {
	res, err := r.Run(ctx, RunRequest{
		Prompt:        prompt,
		Cwd:           dir,
		Model:         model,
		Timeout:       5 * time.Minute,
		DisableTools:  noTools,
		ReadOnlyTools: !noTools,
	})
	if err != nil {
		return "", 0, err
	}
	// Distinguish "took too long" from "had nothing to say". Collapsing the two
	// into an empty string is how a timeout gets recorded as an opinion.
	if res.Aborted {
		return "", res.CostUSD, fmt.Errorf("judge timed out (session %s)", res.SessionID)
	}
	if res.AgentError != "" {
		return "", res.CostUSD, fmt.Errorf("judge failed: %s", res.AgentError)
	}

	if res.Answer != "" {
		return res.Answer, res.CostUSD, nil
	}
	text, err := r.finalAnswer(ctx, res.SessionID, dir)
	return text, res.CostUSD, err
}

// lastAssistantText returns the text of the most recent assistant message that
// actually said something.
func lastAssistantText(msgs []ocMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Info.Role != "assistant" {
			continue
		}
		var text []string
		for _, p := range msgs[i].Parts {
			if p.Type == "text" && strings.TrimSpace(p.Text) != "" {
				text = append(text, p.Text)
			}
		}
		if len(text) > 0 {
			return strings.TrimSpace(strings.Join(text, "\n"))
		}
	}
	return ""
}

// summariseAgentError pulls a readable message out of the agent's error blob.
// Surfacing this matters: a provider refusal that reads as "the model changed
// nothing" turns a configuration problem into a false verdict about the model.
func summariseAgentError(raw json.RawMessage) string {
	var e struct {
		Name string `json:"name"`
		Data struct {
			Message    string `json:"message"`
			StatusCode int    `json:"statusCode"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return strings.TrimSpace(string(raw))
	}
	msg := e.Data.Message
	if msg == "" {
		msg = e.Message
	}
	if msg == "" {
		msg = string(raw)
	}
	if e.Data.StatusCode != 0 {
		return fmt.Sprintf("%s (%d): %s", e.Name, e.Data.StatusCode, msg)
	}
	if e.Name != "" {
		return e.Name + ": " + msg
	}
	return msg
}

// addMCP registers (or replaces) a remote MCP server on the opencode server,
// with this run's agent and workspace in its headers.
func (r *OpenCodeRunner) addMCP(ctx context.Context, dirQ url.Values, m MCPServer) error {
	h := m.headers()
	h["Authorization"] = "Bearer " + m.Token
	body := map[string]any{"name": m.Name, "config": map[string]any{
		"type": "remote", "url": m.URL, "headers": h, "oauth": false, "enabled": true,
	}}
	if err := r.do(ctx, r.client(), http.MethodPost, "/mcp", dirQ, body, nil); err != nil {
		return err
	}
	return r.do(ctx, r.client(), http.MethodPost, "/mcp/"+m.Name+"/connect", dirQ, nil, nil)
}

// dirLocks serialises OpenCode runs that share a working directory.
var dirLocks sync.Map // dir -> *sync.Mutex

func lockDir(dir string) func() {
	m, _ := dirLocks.LoadOrStore(dir, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
