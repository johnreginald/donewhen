package orchestrator

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// Runner executes one attempt at a ticket inside a worktree. Implementations
// wrap a coding agent (OpenCode, Codex, Claude Code, OMP). The orchestrator
// treats them as disposable compute: a runner writes inside the worktree, and
// nothing it produces reaches the repository without passing the gates.
type Runner interface {
	// Name identifies the backend in a verdict.
	Name() string
	// Run dispatches work and blocks until the agent finishes, the context is
	// cancelled, or the request times out.
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

// RunRequest is what a runner needs to do one attempt.
type RunRequest struct {
	// Prompt is the fully built instruction — the context pack, not a ticket title.
	Prompt string
	// Cwd is the worktree. A runner must not write outside it.
	Cwd string
	// Model is "provider/model", e.g. "opencode-go/glm-5.3-flash".
	Model string
	// Timeout bounds the attempt. Zero means DefaultRunTimeout.
	Timeout time.Duration
	// LogPath receives the raw transcript. It exists to be read by a human or a
	// script, never to be piped into a model's context.
	LogPath string
	// SessionID continues an existing session instead of starting cold, so a
	// repair attempt keeps the failed attempt in context.
	SessionID string
	// DisableTools runs the agent with every tool turned off. A question that
	// only needs an opinion should not be given a filesystem to wander around
	// in: doing so is how a judge spends five minutes and returns nothing.
	DisableTools bool
	// ReadOnlyTools allows only the tools needed to look at code, never to
	// change it. Used for review, where a reviewer that can edit is not one.
	ReadOnlyTools bool
	// MCP gives the run Raenil's tools — asking the user, proposing tickets —
	// through whichever means its CLI takes. Nil gives it none.
	MCP *MCPServer
	// Terminal works the run in a live terminal session the user can watch
	// and answer, for runners that can; others run headless as always.
	Terminal bool
	// Title names the terminal window: the ticket key.
	Title string
	// Check runs the ticket's checks between turns of a terminal run.
	Check TerminalCheck
	// OnQuestion is told when a terminal run's agent stops to ask the user.
	OnQuestion func(message string)
}

// terminalRunner is an optional Runner interface for backends that can work
// in a live terminal.
type terminalRunner interface {
	CanRunInTerminal() bool
}

// runsInTerminal reports whether r can honour RunRequest.Terminal.
func runsInTerminal(r Runner) bool {
	t, ok := r.(terminalRunner)
	return ok && t.CanRunInTerminal()
}

// readyChecker is an optional Runner interface for backends that can say
// whether a model is actually usable right now.
//
// A catalog listing is a declaration, not a working path: today a provider was
// declared in config, its model existed in ollama, and the provider package was
// never installed — so everything looked right and nothing could run. The only
// honest check is an end-to-end one through the exact runner and model.
type readyChecker interface {
	Ready(ctx context.Context, model string) error
}

// runnerReady verifies a runner can do the work before a ticket is claimed for
// it. Runners that cannot answer are assumed ready; the check exists to catch
// misconfiguration, not to gate backends that have no way to report.
func runnerReady(ctx context.Context, r Runner, model string) error {
	rc, ok := r.(readyChecker)
	if !ok {
		return nil
	}
	if err := rc.Ready(ctx, model); err != nil {
		return fmt.Errorf("%s cannot run %q: %w", r.Name(), model, err)
	}
	return nil
}

// modelAware is an optional Runner interface for backends that do not take the
// configured model string at face value. Reporting what a runner will actually
// use keeps the log honest: "running codex on opencode-go/glm-5.3-flash" names
// a model Codex never sees.
type modelAware interface {
	EffectiveModel(requested string) string
}

// effectiveModel reports the model a runner will really use.
func effectiveModel(r Runner, requested string) string {
	if m, ok := r.(modelAware); ok {
		return m.EffectiveModel(requested)
	}
	return requested
}

// DefaultRunTimeout bounds an attempt that does not set its own. Every runner
// call is bounded: an unbounded one wedges the daemon with no diagnostic.
const DefaultRunTimeout = 30 * time.Minute

// RunResult is what came back. It deliberately carries no claim about whether
// the work is correct — that is the criteria engine's decision, from evidence.
type RunResult struct {
	// SessionID lets a later attempt fork or continue this one.
	SessionID string
	// Answer is the agent's final message, for runners that report one.
	Answer string
	// Exit is 0 when the agent completed its turn, non-zero otherwise.
	Exit int
	// Aborted is true when the attempt hit its timeout and was killed.
	Aborted bool
	// AgentError is what the agent itself reported — a provider refusal, an
	// entitlement problem, a context overflow. Distinct from a Go error, which
	// means the orchestrator could not talk to the agent at all.
	AgentError string
	// CostUSD and Tokens feed the verdict and the daemon's budget guard.
	CostUSD float64
	Tokens  int
	// CostUnknown means this runner cannot report what it spent — Codex bills
	// against a subscription, not per call. A budget ceiling cannot police what
	// it cannot see, so this is surfaced rather than passed off as zero.
	CostUnknown bool
	// NotionalCostUSD is what the run would have cost at list price when it ran
	// on a subscription. It is recorded, never counted: CostUSD is what the
	// budget guard polices, and a subscription run spends none of it.
	NotionalCostUSD float64
	// Usage splits Tokens by kind. One total misleads: a cache read is a tenth
	// the price of fresh input and most of what a long session consumes.
	Usage TokenUsage
	// Billing says who paid: "subscription" (a logged-in plan), "api" (a key,
	// metered) or "" when the runner cannot tell.
	Billing string
	// Questions holds anything the agent asked. A daemon cannot answer, so these
	// become the escalation payload rather than a hang.
	Questions []string
	// DeniedTools lists tool calls the permission policy refused, so a reviewer
	// can see the agent was stopped rather than merely unlucky.
	DeniedTools []string
	Duration    time.Duration
}

// Environment variables that would switch a CLI from its subscription login to
// metered billing or another provider. Claude and Codex run on the subscription
// only, so a key left in the orchestrator's shell must never reach them. The
// list follows Paperclip's AI_AUTH_ENV_KEYS for the same two CLIs.
var (
	claudeKeyEnv = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL",
		"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"}
	codexKeyEnv = []string{"OPENAI_API_KEY", "CODEX_API_KEY", "OPENAI_BASE_URL"}
)

// subscriptionEnv is the current environment without the named variables.
func subscriptionEnv(drop []string) []string {
	env := os.Environ()
	out := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(drop, name) {
			out = append(out, kv)
		}
	}
	return out
}

// TokenUsage is a run's tokens by kind, for runners that report the split.
type TokenUsage struct {
	Input         int `json:"input"`
	CacheRead     int `json:"cache_read"`
	CacheCreation int `json:"cache_creation"`
	Output        int `json:"output"`
}

// Total is every token the model processed.
func (u TokenUsage) Total() int { return u.Input + u.CacheRead + u.CacheCreation + u.Output }

// splitModel turns "provider/model" into its parts. A bare name has no provider.
func splitModel(s string) (provider, model string) {
	if i := strings.Index(s, "/"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// readyCache remembers which (runner, model) pairs have been proven usable, so
// a daemon working a queue pays for the check once rather than per ticket.
type readyCache struct {
	mu   sync.Mutex
	seen map[string]error
}

func (c *readyCache) once(key string, check func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil {
		c.seen = map[string]error{}
	}
	if err, ok := c.seen[key]; ok {
		return err
	}
	err := check()
	// Only a success is remembered. A transient failure — the server still
	// starting, the tailnet briefly down — must not poison the rest of the run.
	if err == nil {
		c.seen[key] = nil
	}
	return err
}
