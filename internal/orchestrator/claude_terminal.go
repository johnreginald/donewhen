package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CanRunInTerminal implements terminalRunner.
func (r *ClaudeRunner) CanRunInTerminal() bool { return true }

// runTerminal works a ticket in interactive Claude inside a tmux session. It
// has the same login, model, MCP servers, tools and permission mode as a
// headless run; only the user can now see it and type into it.
//
// Measured (claude 2.1.283): the Stop hook's JSON carries session_id and
// last_assistant_message; a bracketed paste into the TUI arrives as one
// message; a config directory's first start shows theme and onboarding
// screens unless .claude.json says they were seen; trusting a folder trusts
// everything under it.
func (r *ClaudeRunner) runTerminal(ctx context.Context, req RunRequest) (RunResult, error) {
	res, err := terminalRun(ctx, req, func(runDir string, sess *terminalSession) (string, func(), error) {
		mcpConfig := filepath.Join(runDir, "mcp.json")
		cleanup := func() {}
		switch {
		case req.MCP != nil:
			path, c, err := claudeMCPConfig(*req.MCP)
			if err != nil {
				return "", nil, err
			}
			mcpConfig, cleanup = path, c
		case r.MCPConfig != "":
			mcpConfig = r.MCPConfig
		default:
			if err := os.WriteFile(mcpConfig, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
				return "", nil, err
			}
		}

		// The Stop hook appends each finished turn to the turns file.
		settings, _ := json.Marshal(map[string]any{"hooks": map[string]any{"Stop": []any{map[string]any{
			"hooks": []any{map[string]any{"type": "command",
				"command": "cat >> " + shellQuote(sess.TurnsPath()) + "; echo >> " + shellQuote(sess.TurnsPath())}},
		}}}})
		settingsPath := filepath.Join(runDir, "claude-settings.json")
		if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
			cleanup()
			return "", nil, err
		}

		env, err := r.terminalEnv(req.Cwd)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		// The prompt goes first: --tools and --mcp-config take several
		// values and would swallow a positional after them.
		script := terminalScript(runDir, env, req.Cwd, r.bin(), r.interactiveArgs(req, mcpConfig, settingsPath))
		return script, cleanup, nil
	})
	if r.OAuthToken != "" {
		res.Billing = "subscription"
	}
	return res, err
}

// terminalRun is the part of a terminal run every harness shares: the run
// directory, the session, the turn loop and the result. build writes the
// harness's launch script.
func terminalRun(ctx context.Context, req RunRequest,
	build func(runDir string, sess *terminalSession) (script string, cleanup func(), err error)) (RunResult, error) {
	start := time.Now()
	res := RunResult{CostUnknown: true}
	ctx, cancel := context.WithTimeout(ctx, TerminalTimeout)
	defer cancel()

	runDir := filepath.Dir(req.LogPath)
	if req.LogPath == "" {
		tmp, err := os.MkdirTemp("", "raenil-terminal-")
		if err != nil {
			return res, err
		}
		defer os.RemoveAll(tmp)
		runDir = tmp
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return res, err
	}
	logf := terminalLogger(req.LogPath)
	if err := os.WriteFile(filepath.Join(runDir, "prompt.md"), []byte(req.Prompt), 0o600); err != nil {
		return res, err
	}

	sess := &terminalSession{Name: sessionName(req.Title), Title: req.Title, Dir: req.Cwd, RunDir: runDir, Log: logf}
	script, cleanup, err := build(runDir, sess)
	if err != nil {
		return res, err
	}
	defer cleanup()
	defer os.Remove(filepath.Join(runDir, "terminal.env"))

	if err := sess.Start(ctx, script); err != nil {
		return res, err
	}
	defer sess.Close(context.WithoutCancel(ctx))
	logf("working in a terminal: tmux attach -t %s", sess.Name)

	turn, err := runTerminalTurns(ctx, sess, req.Check, func(msg string) {
		logf("waiting for you in the terminal: %s", firstLine(msg))
		if req.OnQuestion != nil {
			req.OnQuestion(msg)
		}
	})
	res.Duration = time.Since(start)
	res.SessionID, res.Answer = turn.SessionID, turn.Message
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Aborted, res.Exit = true, 124
		return res, nil
	case errors.Is(err, errSessionEnded):
		// Someone closed it or the agent exited: what is in the worktree is
		// what gets checked.
		logf("the terminal session ended")
		return res, nil
	case err != nil:
		return res, err
	}
	return res, nil
}

// terminalScript is the launch script: it sources the environment file —
// which holds the login — and removes it before anything else runs, then
// starts bin with the prompt first and args after.
func terminalScript(runDir, env, cwd, bin string, args []string) string {
	envPath := filepath.Join(runDir, "terminal.env")
	_ = os.WriteFile(envPath, []byte(env), 0o600)
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return "#!/bin/sh\n" +
		". " + shellQuote(envPath) + "\nrm -f " + shellQuote(envPath) + "\n" +
		"cd " + shellQuote(cwd) + " || exit 1\n" +
		"exec " + shellQuote(bin) + " \"$(cat " + shellQuote(filepath.Join(runDir, "prompt.md")) + ")\" " + strings.Join(quoted, " ") + "\n"
}

// baseTerminalEnv is the host's PATH and HOME with the named variables
// cleared, as a sourced file.
func baseTerminalEnv(clear []string) string {
	var b strings.Builder
	for _, k := range []string{"PATH", "HOME", "LANG", "TMPDIR"} {
		if v := os.Getenv(k); v != "" {
			fmt.Fprintf(&b, "export %s=%s\n", k, shellQuote(v))
		}
	}
	for _, k := range clear {
		fmt.Fprintf(&b, "unset %s\n", k)
	}
	return b.String()
}

// interactiveArgs are args() for the TUI: the same session settings, without
// print mode.
func (r *ClaudeRunner) interactiveArgs(req RunRequest, mcpConfig, settingsPath string) []string {
	head := r.args(req, mcpConfig)
	out := make([]string, 0, len(head)+2)
	for i := 0; i < len(head); i++ {
		switch head[i] {
		case "-p", "--verbose":
			continue
		case "--output-format":
			i++ // and its value
			continue
		case "--max-turns", "--max-budget-usd":
			i++ // print mode only
			continue
		}
		out = append(out, head[i])
	}
	return append(out, "--settings", settingsPath)
}

// terminalEnv is the environment file a terminal run sources: the host's
// PATH and HOME, the metered-billing variables cleared, and Raenil's own
// login when there is one.
func (r *ClaudeRunner) terminalEnv(worktree string) (string, error) {
	var b strings.Builder
	b.WriteString(baseTerminalEnv(claudeKeyEnv))
	if r.OAuthToken != "" {
		dir := r.ConfigDir
		if dir == "" {
			return "", errors.New("a terminal run on Raenil's Claude login needs its config directory")
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		if err := prepareClaudeConfig(dir, worktree); err != nil {
			return "", fmt.Errorf("prepare the Claude config for a terminal: %w", err)
		}
		fmt.Fprintf(&b, "export CLAUDE_CONFIG_DIR=%s\nexport CLAUDE_CODE_OAUTH_TOKEN=%s\n",
			shellQuote(dir), shellQuote(r.OAuthToken))
	} else {
		b.WriteString("unset CLAUDE_CONFIG_DIR CLAUDE_CODE_OAUTH_TOKEN\n")
	}
	return b.String(), nil
}

// prepareClaudeConfig marks a Claude config directory's first-start screens
// as seen and trusts the worktree, so a terminal session opens straight on
// the work. Trust is looked up at the git root, and every worktree is one, so
// each gets its own entry. It only adds keys; everything else is kept.
func prepareClaudeConfig(dir, trust string) error {
	path := filepath.Join(dir, ".claude.json")
	cfg := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cfg); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	changed := false
	setDefault := func(k string, v any) {
		if _, ok := cfg[k]; !ok {
			cfg[k], changed = v, true
		}
	}
	setDefault("hasCompletedOnboarding", true)
	setDefault("lastOnboardingVersion", "2.1.283")
	setDefault("theme", "dark")
	setDefault("hasSeenAutoDefaultNotice", true)
	setDefault("hasSeenAutoModeOutsideReadPrompt", true)
	setDefault("autoModeClassifierBillingNoticeAcknowledgedAt", time.Now().UnixMilli())

	projects, _ := cfg["projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
		cfg["projects"] = projects
	}
	// Claude checks the folder's real path: /var/folders is /private/var/folders.
	paths := []string{trust}
	if real, err := filepath.EvalSymlinks(trust); err == nil && real != trust {
		paths = append(paths, real)
	}
	for _, path := range paths {
		p, _ := projects[path].(map[string]any)
		if p == nil {
			p = map[string]any{}
			projects[path] = p
		}
		if v, _ := p["hasTrustDialogAccepted"].(bool); !v {
			p["hasTrustDialogAccepted"], changed = true, true
		}
	}
	if !changed {
		return nil
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".raenil-tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// terminalLogger appends the host's notes about a terminal run to its log,
// which Raenil streams onto the ticket.
func terminalLogger(path string) func(string, ...any) {
	return func(format string, args ...any) {
		if path == "" {
			return
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	}
}

// firstLine is a message's first non-empty line, cut short.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if len(l) > 160 {
				l = l[:160] + "…"
			}
			return l
		}
	}
	return ""
}
