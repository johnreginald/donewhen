package orchestrator

import (
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

// CodexRunner drives the Codex CLI's non-interactive mode.
//
// Unlike OpenCode this is process-shaped rather than server-shaped: a run is a
// child process, and completion is its exit. That removes the completion-polling
// race entirely, at the cost of the session control an HTTP API gives.
type CodexRunner struct {
	// Bin is the executable. Empty means "codex" on PATH.
	Bin string
	// Sandbox overrides the policy for work runs. Empty means workspace-write,
	// which is the least authority a worker can have and still edit files.
	Sandbox string
}

// Name implements Runner.
func (r *CodexRunner) Name() string { return "codex" }

func (r *CodexRunner) bin() string {
	if r.Bin != "" {
		return r.Bin
	}
	return "codex"
}

func (r *CodexRunner) sandbox() string {
	if r.Sandbox != "" {
		return r.Sandbox
	}
	return "workspace-write"
}

// EffectiveModel reports what Codex will actually run with. Anything not
// addressed to codex/ is ignored in favour of its own configuration.
func (r *CodexRunner) EffectiveModel(requested string) string {
	if provider, model := splitModel(requested); provider == "codex" && model != "" && model != "default" {
		return model
	}
	return "its own configured model"
}

// Available reports whether the CLI is installed and logged in, so a missing
// Codex is a clear message rather than an exec error mid-ticket.
func (r *CodexRunner) Available(ctx context.Context) error {
	if _, err := exec.LookPath(r.bin()); err != nil {
		return fmt.Errorf("codex is not on PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := r.capture(ctx, "", "login", "status")
	if err != nil {
		return fmt.Errorf("codex login status: %w", err)
	}
	if !strings.Contains(strings.ToLower(out), "logged in") {
		return fmt.Errorf("codex is not logged in: %s", strings.TrimSpace(out))
	}
	return nil
}

func (r *CodexRunner) capture(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.bin(), args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Run implements Runner.
func (r *CodexRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultRunTimeout
	}
	sandbox := r.sandbox()
	if req.ReadOnlyTools {
		sandbox = "read-only"
	}
	if req.DisableTools {
		// Codex has no "no tools at all" mode; read-only is the closest, and a
		// reviewer that cannot write is what the caller actually wanted.
		sandbox = "read-only"
	}
	answer, res, err := r.run(ctx, req, sandbox, timeout)
	_ = answer
	return res, err
}

// run executes one Codex invocation and returns its final message alongside the
// result, so Ask and Run share exactly one code path.
func (r *CodexRunner) run(ctx context.Context, req RunRequest, sandbox string, timeout time.Duration) (string, RunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	res := RunResult{CostUnknown: true}

	lastMsg, err := os.CreateTemp("", "codex-last-*.txt")
	if err != nil {
		return "", res, err
	}
	lastPath := lastMsg.Name()
	lastMsg.Close()
	defer os.Remove(lastPath)

	args := []string{
		"exec",
		"-C", req.Cwd,
		"--skip-git-repo-check",
		"--sandbox", sandbox,
		"--output-last-message", lastPath,
		"--json",
		"--color", "never",
	}
	// Only a model explicitly addressed to Codex is passed on, and "default"
	// means its own configured choice rather than a model literally named that.
	//
	// The configured model is usually an OpenCode one ("opencode-go/glm-5.3-flash")
	// which means nothing here: handing it over makes Codex fail on an unknown
	// model instead of using the one it is already set up with.
	if provider, model := splitModel(req.Model); provider == "codex" && model != "" && model != "default" {
		args = append(args, "-m", model)
	}
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, r.bin(), args...)
	cmd.Dir = req.Cwd
	// Never inherit a live stdin: a child that blocks on it hangs with no output,
	// which is exactly how the OpenCode CLI wedged.
	cmd.Stdin = nil

	// Always capture output. Discarding it when no log was requested is how a
	// failure becomes "codex returned nothing" with no way to find out why.
	logPath := req.LogPath
	if logPath == "" {
		tmp, terr := os.CreateTemp("", "codex-run-*.log")
		if terr != nil {
			return "", res, terr
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
	res.Tokens = codexTokens(logPath)

	if b, rerr := os.ReadFile(lastPath); rerr == nil {
		answerOut := strings.TrimSpace(string(b))
		switch {
		case ctx.Err() == context.DeadlineExceeded:
			res.Aborted, res.Exit = true, 124
			return answerOut, res, nil
		case runErr == nil:
			res.Exit = 0
			return answerOut, res, nil
		}
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			res.Exit = ee.ExitCode()
			res.AgentError = fmt.Sprintf("codex exited %d: %s", ee.ExitCode(), codexTail(logPath))
			return answerOut, res, nil
		}
		return answerOut, res, runErr
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.Aborted, res.Exit = true, 124
		return "", res, nil
	}
	if runErr != nil {
		res.Exit = 1
		return "", res, fmt.Errorf("%w: %s", runErr, codexTail(logPath))
	}
	return "", res, nil
}

// codexTail returns the last few meaningful lines of a run's output, for an
// error message that names a cause instead of reporting silence.
func codexTail(logPath string) string {
	b, err := os.ReadFile(logPath)
	if err != nil {
		return "(no output captured)"
	}
	var keep []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.Contains(l, "rmcp::transport") {
			continue // MCP auth noise, not this run's problem
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

// Ask implements Asker: a one-shot question in a scratch directory, read-only.
func (r *CodexRunner) Ask(ctx context.Context, model, prompt string) (string, float64, error) {
	dir, err := os.MkdirTemp("", "codex-ask-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(dir)
	return r.askIn(ctx, model, prompt, dir)
}

// AskIn implements Asker: a question answered from inside a real directory, with
// the filesystem readable and nothing writable.
func (r *CodexRunner) AskIn(ctx context.Context, model, prompt, cwd string) (string, float64, error) {
	return r.askIn(ctx, model, prompt, cwd)
}

func (r *CodexRunner) askIn(ctx context.Context, model, prompt, dir string) (string, float64, error) {
	answer, res, err := r.run(ctx, RunRequest{
		Prompt: prompt,
		Cwd:    dir,
		Model:  model,
	}, "read-only", 5*time.Minute)
	if err != nil {
		return "", 0, err
	}
	if res.Aborted {
		return "", 0, fmt.Errorf("codex timed out after 5m")
	}
	if answer == "" {
		if res.AgentError != "" {
			return "", 0, fmt.Errorf("codex returned nothing: %s", res.AgentError)
		}
		return "", 0, fmt.Errorf("codex returned nothing (exit %d)", res.Exit)
	}
	// Cost is not reported: Codex bills against a subscription rather than
	// per call. Zero here is a fact, not an estimate — see RunResult.CostUnknown.
	return answer, 0, nil
}

var (
	_ Runner = (*CodexRunner)(nil)
	_ Asker  = (*CodexRunner)(nil)
)

// codexTokens reads the token usage Codex reports on its final JSONL event.
// Cost is still unknown — a subscription has no per-call price — but tokens are
// real and worth recording.
func codexTokens(logPath string) int {
	b, err := os.ReadFile(logPath)
	if err != nil {
		return 0
	}
	total := 0
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, `"turn.completed"`) {
			continue
		}
		var ev struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(line), &ev) == nil {
			total += ev.Usage.InputTokens + ev.Usage.OutputTokens
		}
	}
	return total
}
