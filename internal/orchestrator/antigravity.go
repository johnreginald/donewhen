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

// AntigravityRunner drives Google Antigravity's CLI (agy) in print mode, on
// the Google subscription the machine is signed in with.
//
// Measured (agy 1.2.5):
//   - -p takes the prompt as its value; the prompt must be attached to it.
//   - stream-json ends with {"event":"result","result":{conversation_id,
//     status, response, usage{input_tokens, output_tokens, thinking_tokens,
//     cache_read_tokens, total_tokens}}}.
//   - --conversation <id> continues a conversation.
//   - --print-timeout defaults to 5m, too short for a ticket.
//   - --sandbox blocked the Go toolchain yet still wrote to /tmp: it is not
//     isolation, so it is not used; isolation is the container's job.
type AntigravityRunner struct {
	// Bin is the executable. Empty means "agy" on PATH.
	Bin string
}

// Name implements Runner.
func (r *AntigravityRunner) Name() string { return "antigravity" }

func (r *AntigravityRunner) bin() string {
	if r.Bin != "" {
		return r.Bin
	}
	return "agy"
}

// Available reports whether agy is installed.
func (r *AntigravityRunner) Available(ctx context.Context) error {
	if _, err := exec.LookPath(r.bin()); err != nil {
		return fmt.Errorf("agy is not on PATH: %w", err)
	}
	// agy models answers in a second and says when nobody is signed in; a
	// print run would sit waiting for a browser sign-in instead.
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.bin(), "models")
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if strings.Contains(strings.ToLower(string(out)), "sign in") {
		return errors.New("agy is not signed in: run agy once on this machine and sign in with Google")
	}
	if err != nil {
		return fmt.Errorf("agy models: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Run implements Runner.
func (r *AntigravityRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultRunTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+time.Minute)
	defer cancel()
	start := time.Now()
	res := RunResult{CostUnknown: true, Billing: "subscription"}

	prompt := req.Prompt
	if !req.DisableTools && !req.ReadOnlyTools {
		// agy has no deny rules to fence folders off; say it.
		prompt += protectedPrompt
	}
	args := []string{
		"--output-format", "stream-json",
		"--print-timeout", timeout.String(),
		"--disable-slash-commands",
	}
	switch {
	case req.DisableTools || req.ReadOnlyTools:
		args = append(args, "--mode", "plan")
	default:
		args = append(args, "--dangerously-skip-permissions")
	}
	if provider, model := splitModel(req.Model); (provider == "antigravity" || provider == "agy") && model != "" && model != "default" {
		args = append(args, "--model", model)
	}
	if req.SessionID != "" {
		args = append(args, "--conversation", req.SessionID)
	}
	args = append(args, "--print="+prompt)

	cmd := exec.CommandContext(ctx, r.bin(), args...)
	cmd.Dir = req.Cwd
	cmd.Stdin = nil
	logPath := req.LogPath
	if logPath == "" {
		tmp, err := os.CreateTemp("", "agy-run-*.jsonl")
		if err != nil {
			return res, err
		}
		logPath = tmp.Name()
		tmp.Close()
		defer os.Remove(logPath)
	} else {
		_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	}
	before := fileSize(logPath)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return res, err
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile

	runErr := cmd.Run()
	res.Duration = time.Since(start)
	result, found := agyResult(logPath, before)
	if found {
		res.SessionID, res.Answer = result.ConversationID, strings.TrimSpace(result.Response)
		u := result.Usage
		res.Usage = TokenUsage{Input: u.InputTokens, Output: u.OutputTokens + u.ThinkingTokens, CacheRead: u.CacheReadTokens}
		res.Tokens = res.Usage.Total()
	}
	if res.SessionID == "" {
		res.SessionID = req.SessionID
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Aborted, res.Exit = true, 124
	case runErr != nil:
		var ee *exec.ExitError
		if !errors.As(runErr, &ee) {
			return res, fmt.Errorf("agy: %w", runErr)
		}
		res.Exit = ee.ExitCode()
		res.AgentError = fmt.Sprintf("agy exited %d: %s", res.Exit, tailOf(logPath, 600))
	case found && !strings.EqualFold(result.Status, "SUCCESS"):
		res.Exit = 1
		res.AgentError = "agy finished with status " + result.Status
	case !found:
		res.Exit = 1
		res.AgentError = "agy reported no result: " + tailOf(logPath, 600)
	}
	return res, nil
}

// Ask implements Asker: a question answered from the conversation alone.
func (r *AntigravityRunner) Ask(ctx context.Context, model, prompt string) (string, float64, error) {
	res, err := r.Run(ctx, RunRequest{Prompt: prompt, Model: model, DisableTools: true, Timeout: 5 * time.Minute})
	if err != nil {
		return "", 0, err
	}
	if res.AgentError != "" {
		return res.Answer, 0, errors.New(res.AgentError)
	}
	return res.Answer, 0, nil
}

// agyUsage and agyFinal are the result event's shape.
type agyUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ThinkingTokens  int `json:"thinking_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
}

type agyFinal struct {
	ConversationID string   `json:"conversation_id"`
	Status         string   `json:"status"`
	Response       string   `json:"response"`
	Usage          agyUsage `json:"usage"`
}

// agyResult reads the last result event written after offset.
func agyResult(path string, offset int64) (agyFinal, bool) {
	var out agyFinal
	f, err := os.Open(path)
	if err != nil {
		return out, false
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return out, false
	}
	found := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var ev struct {
			Event  string   `json:"event"`
			Result agyFinal `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Event == "result" {
			out, found = ev.Result, true
		}
	}
	return out, found
}

func fileSize(path string) int64 {
	if st, err := os.Stat(path); err == nil {
		return st.Size()
	}
	return 0
}

// tailOf is the last n bytes of a file, for an error message.
func tailOf(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return strings.TrimSpace(string(b))
}

// AntigravityModels lists the models agy offers this login.
func AntigravityModels(ctx context.Context, r *AntigravityRunner) []string {
	out, err := exec.CommandContext(ctx, r.bin(), "models").Output()
	if err != nil {
		return nil
	}
	var models []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(line, "Fetching") {
			continue
		}
		models = append(models, f[0])
	}
	return models
}
