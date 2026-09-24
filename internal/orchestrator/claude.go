package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeRunner drives Claude Code's print mode on the subscription the Mac is
// logged in with — never an API key.
//
// Like Codex it is process-shaped: a run is a child process and completion is
// its exit. The transcript is stream-json, whose final result event carries the
// session, the answer, the usage and the tool calls the permission policy
// refused.
//
// Four things were measured, not assumed (claude 2.1.281):
//   - Without --setting-sources project a run loads the user's personal
//     ~/.claude settings and hooks. A worker must not inherit those.
//   - --tools and --mcp-config take several values and swallow a positional
//     prompt after them, so the prompt goes on stdin.
//   - Under acceptEdits, writes outside the working directory are refused and
//     reported in permission_denials; file commands inside it are allowed.
//   - Read-only shell commands run even when Bash is not allowlisted.
type ClaudeRunner struct {
	// Bin is the executable. Empty means "claude" on PATH.
	Bin string
	// AllowedTools extends what a work run may use without asking, as Claude
	// Code permission rules, e.g. "Bash(go test *)". Anything else that needs
	// approval is refused, since print mode has nobody to ask.
	AllowedTools []string
	// MCPConfig is an MCP config file for work runs. Empty gives the worker no
	// MCP servers at all; either way the user's own servers are never loaded.
	MCPConfig string
	// MaxBudgetUSD is passed as --max-budget-usd. Zero is no cap. Unlike the
	// daemon's hourly guard it would act during the run; how the CLI applies it
	// on a subscription is not yet verified.
	MaxBudgetUSD float64

	ready readyCache
}

// claudeWorkTools are what an editing run has.
var claudeWorkTools = []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash"}

// claudeWorkAllowed are the tools a work run may use without asking. Bash is
// deliberately absent: allowlisting it bare approves every command, and the
// live test watched a worker write outside its worktree that way. Without it,
// acceptEdits still lets file and read-only commands run inside the worktree;
// anything more is granted per repo as a pattern such as "Bash(go test *)".
var claudeWorkAllowed = []string{"Read", "Edit", "Write", "Glob", "Grep"}

// claudeReadTools are what a reviewer may use: enough to look, never to change.
var claudeReadTools = []string{"Read", "Glob", "Grep"}

// Name implements Runner.
func (r *ClaudeRunner) Name() string { return "claude" }

func (r *ClaudeRunner) bin() string {
	if r.Bin != "" {
		return r.Bin
	}
	return "claude"
}

// EffectiveModel reports what Claude will actually run with. Anything not
// addressed to claude/ is ignored in favour of its own configured model.
func (r *ClaudeRunner) EffectiveModel(requested string) string {
	if provider, model := splitModel(requested); provider == "claude" && model != "" && model != "default" {
		return model
	}
	return "its own configured model"
}

// claudeAuth is the part of `claude auth status` a runner cares about.
type claudeAuth struct {
	LoggedIn   bool   `json:"loggedIn"`
	AuthMethod string `json:"authMethod"`
}

// Available reports whether the CLI is installed and logged in to a
// subscription. An API-key login is refused: Claude runs on the subscription,
// and a key would quietly turn every run into real spend.
func (r *ClaudeRunner) Available(ctx context.Context) error {
	if _, err := exec.LookPath(r.bin()); err != nil {
		return fmt.Errorf("claude is not on PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.bin(), "auth", "status")
	cmd.Env = subscriptionEnv(claudeKeyEnv)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("claude auth status: %w", err)
	}
	var a claudeAuth
	if err := json.Unmarshal(out, &a); err != nil {
		return fmt.Errorf("claude auth status: unreadable output: %s", trunc(strings.TrimSpace(string(out)), 200))
	}
	if !a.LoggedIn {
		return fmt.Errorf("claude is not logged in: run `claude auth login`")
	}
	if a.AuthMethod != "claude.ai" {
		return fmt.Errorf("claude is logged in with %q, not a Claude subscription: run `claude auth login`", a.AuthMethod)
	}
	return nil
}

// Ready proves Claude is installed, on a subscription, and will answer with this
// model, before a ticket is claimed for it.
func (r *ClaudeRunner) Ready(ctx context.Context, model string) error {
	return r.ready.once(model, func() error {
		if err := r.Available(ctx); err != nil {
			return err
		}
		answer, _, err := r.Ask(ctx, model, "Reply with exactly: ok")
		if err != nil {
			return err
		}
		if strings.TrimSpace(answer) == "" {
			return fmt.Errorf("claude produced no output")
		}
		return nil
	})
}

// Run implements Runner.
func (r *ClaudeRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	_, res, err := r.run(ctx, req)
	return res, err
}

// args builds the command line. The prompt is not in it: it goes on stdin.
func (r *ClaudeRunner) args(req RunRequest, mcpConfig string) []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--setting-sources", "project",
		"--strict-mcp-config",
		"--mcp-config", mcpConfig,
	}
	switch {
	case req.DisableTools:
		args = append(args, "--tools", "")
	case req.ReadOnlyTools:
		args = append(args, "--tools", strings.Join(claudeReadTools, ","),
			"--allowedTools", strings.Join(claudeReadTools, ","))
	default:
		allowed := append(append([]string{}, claudeWorkAllowed...), r.AllowedTools...)
		args = append(args, "--permission-mode", "acceptEdits",
			"--tools", strings.Join(claudeWorkTools, ","),
			"--allowedTools", strings.Join(allowed, ","))
	}
	if provider, model := splitModel(req.Model); provider == "claude" && model != "" && model != "default" {
		args = append(args, "--model", model)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	if r.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", r.MaxBudgetUSD))
	}
	return args
}

// run executes one invocation and returns the final answer alongside the
// result, so Ask and Run share one code path.
func (r *ClaudeRunner) run(ctx context.Context, req RunRequest) (string, RunResult, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultRunTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	res := RunResult{}

	// Only work runs get the configured MCP servers; a judge needs none.
	mcpConfig := r.MCPConfig
	if mcpConfig == "" || req.DisableTools || req.ReadOnlyTools {
		f, err := os.CreateTemp("", "claude-mcp-*.json")
		if err != nil {
			return "", res, err
		}
		f.WriteString(`{"mcpServers":{}}`)
		f.Close()
		defer os.Remove(f.Name())
		mcpConfig = f.Name()
	}

	cmd := exec.CommandContext(ctx, r.bin(), r.args(req, mcpConfig)...)
	cmd.Dir = req.Cwd
	cmd.Env = subscriptionEnv(claudeKeyEnv)
	cmd.Stdin = strings.NewReader(req.Prompt)

	// Always capture output: discarding it is how a failure becomes "claude
	// returned nothing" with no way to find out why.
	logPath := req.LogPath
	if logPath == "" {
		tmp, err := os.CreateTemp("", "claude-run-*.jsonl")
		if err != nil {
			return "", res, err
		}
		logPath = tmp.Name()
		tmp.Close()
		defer os.Remove(logPath)
	} else {
		_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return "", res, err
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile

	runErr := cmd.Run()
	res.Duration = time.Since(start)

	t, perr := parseClaudeTranscript(logPath)
	if perr == nil {
		t.apply(&res)
	}

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res.Aborted, res.Exit = true, 124
		return t.answer, res, nil
	case runErr == nil && !t.isError:
		return t.answer, res, nil
	}
	var ee *exec.ExitError
	switch {
	case errors.As(runErr, &ee):
		res.Exit = ee.ExitCode()
	case runErr != nil:
		return t.answer, res, fmt.Errorf("%w: %s", runErr, claudeTail(logPath))
	default:
		res.Exit = 1 // exited cleanly but reported an error
	}
	res.AgentError = t.errorText()
	if res.AgentError == "" {
		res.AgentError = fmt.Sprintf("claude exited %d: %s", res.Exit, claudeTail(logPath))
	}
	return t.answer, res, nil
}

// claudeTranscript is what a stream-json log says about a finished run.
type claudeTranscript struct {
	sessionID    string
	apiKeySource string // "none" means the subscription login
	answer       string
	isError      bool
	subtype      string // success | error_max_turns | error_max_budget_usd | ...
	costUSD      float64
	usage        TokenUsage
	denied       []string
	sawResult    bool
}

// parseClaudeTranscript reads a stream-json log. Lines that are not JSON —
// stderr shares the file — are skipped rather than failing the parse.
func parseClaudeTranscript(path string) (claudeTranscript, error) {
	var t claudeTranscript
	f, err := os.Open(path)
	if err != nil {
		return t, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev struct {
			Type         string  `json:"type"`
			Subtype      string  `json:"subtype"`
			SessionID    string  `json:"session_id"`
			APIKeySource string  `json:"apiKeySource"`
			IsError      bool    `json:"is_error"`
			Result       string  `json:"result"`
			TotalCostUSD float64 `json:"total_cost_usd"`
			ModelUsage   map[string]struct {
				InputTokens              int `json:"inputTokens"`
				OutputTokens             int `json:"outputTokens"`
				CacheReadInputTokens     int `json:"cacheReadInputTokens"`
				CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
			} `json:"modelUsage"`
			PermissionDenials []struct {
				ToolName  string          `json:"tool_name"`
				ToolInput json.RawMessage `json:"tool_input"`
			} `json:"permission_denials"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch {
		case ev.Type == "system" && ev.Subtype == "init":
			t.sessionID, t.apiKeySource = ev.SessionID, ev.APIKeySource
		case ev.Type == "result":
			t.sawResult = true
			if ev.SessionID != "" {
				t.sessionID = ev.SessionID
			}
			t.answer = strings.TrimSpace(ev.Result)
			t.isError, t.subtype, t.costUSD = ev.IsError, ev.Subtype, ev.TotalCostUSD
			// modelUsage, not the top-level usage: the top level leaves out
			// what subagents spent.
			t.usage = TokenUsage{}
			for _, m := range ev.ModelUsage {
				t.usage.Input += m.InputTokens
				t.usage.Output += m.OutputTokens
				t.usage.CacheRead += m.CacheReadInputTokens
				t.usage.CacheCreation += m.CacheCreationInputTokens
			}
			t.denied = t.denied[:0]
			for _, d := range ev.PermissionDenials {
				t.denied = append(t.denied, d.ToolName+" "+trunc(claudeToolInput(d.ToolInput), 160))
			}
		}
	}
	return t, sc.Err()
}

// claudeToolInput shows a refused call by its command or path when it has one.
func claudeToolInput(raw json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(raw, &in) == nil {
		for _, k := range []string{"command", "file_path", "path", "pattern"} {
			if s, ok := in[k].(string); ok {
				return s
			}
		}
	}
	return string(raw)
}

// subscription reports whether the run was paid for by the logged-in plan.
func (t claudeTranscript) subscription() bool { return t.apiKeySource == "none" }

func (t claudeTranscript) apply(res *RunResult) {
	res.SessionID = t.sessionID
	res.Usage = t.usage
	res.Tokens = t.usage.Total()
	res.DeniedTools = append([]string(nil), t.denied...)
	switch {
	case t.subscription():
		res.Billing, res.NotionalCostUSD = "subscription", t.costUSD
	case t.apiKeySource != "":
		res.Billing, res.CostUSD = "api", t.costUSD
	default:
		// No init event: nothing says who paid, so it is counted, not hidden.
		res.CostUSD = t.costUSD
	}
}

func (t claudeTranscript) errorText() string {
	if !t.isError && t.subtype == "success" {
		return ""
	}
	switch {
	case !t.sawResult:
		return ""
	case t.answer != "":
		return fmt.Sprintf("claude %s: %s", t.subtype, trunc(t.answer, 300))
	default:
		return "claude " + t.subtype
	}
}

// claudeTail returns the last few non-JSON lines of a run's log — where the CLI
// writes its own errors — for a message that names a cause.
func claudeTail(logPath string) string {
	b, err := os.ReadFile(logPath)
	if err != nil {
		return "(no output captured)"
	}
	var keep []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "{") {
			continue
		}
		keep = append(keep, l)
	}
	if len(keep) == 0 {
		return "(no output captured)"
	}
	if len(keep) > 4 {
		keep = keep[len(keep)-4:]
	}
	return trunc(strings.Join(keep, " | "), 400)
}

// Ask implements Asker: a one-shot opinion with no tools at all.
func (r *ClaudeRunner) Ask(ctx context.Context, model, prompt string) (string, float64, error) {
	dir, err := os.MkdirTemp("", "claude-ask-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(dir)
	return r.ask(ctx, RunRequest{Prompt: prompt, Cwd: dir, Model: model, DisableTools: true})
}

// AskIn implements Asker: a question answered from inside a real directory,
// with the files readable and nothing writable.
func (r *ClaudeRunner) AskIn(ctx context.Context, model, prompt, cwd string) (string, float64, error) {
	return r.ask(ctx, RunRequest{Prompt: prompt, Cwd: cwd, Model: model, ReadOnlyTools: true})
}

func (r *ClaudeRunner) ask(ctx context.Context, req RunRequest) (string, float64, error) {
	req.Timeout = 5 * time.Minute
	answer, res, err := r.run(ctx, req)
	if err != nil {
		return "", 0, err
	}
	if res.Aborted {
		return "", 0, fmt.Errorf("claude timed out after 5m")
	}
	if answer == "" || res.AgentError != "" {
		if res.AgentError != "" {
			return "", 0, fmt.Errorf("claude returned nothing: %s", res.AgentError)
		}
		return "", 0, fmt.Errorf("claude returned nothing (exit %d)", res.Exit)
	}
	// A subscription run costs nothing against the budget; see NotionalCostUSD.
	return answer, res.CostUSD, nil
}

var (
	_ Runner = (*ClaudeRunner)(nil)
	_ Asker  = (*ClaudeRunner)(nil)
)
