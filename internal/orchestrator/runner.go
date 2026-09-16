package orchestrator

import (
	"context"
	"strings"
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
	// Questions holds anything the agent asked. A daemon cannot answer, so these
	// become the escalation payload rather than a hang.
	Questions []string
	// DeniedTools lists tool calls the permission policy refused, so a reviewer
	// can see the agent was stopped rather than merely unlucky.
	DeniedTools []string
	Duration    time.Duration
}

// splitModel turns "provider/model" into its parts. A bare name has no provider.
func splitModel(s string) (provider, model string) {
	if i := strings.Index(s, "/"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}
