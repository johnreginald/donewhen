package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CanRunInTerminal implements terminalRunner.
func (r *CodexRunner) CanRunInTerminal() bool { return true }

// runTerminal works a ticket in the Codex TUI inside a tmux session, with the
// same login, sandbox, model and MCP servers as a headless run, and approvals
// asked of the user when Codex wants to step outside the sandbox.
//
// Measured (codex-cli 0.157.0): notify runs a program with the turn's JSON as
// its last argument, carrying thread-id and last-assistant-message; Codex
// also titles each session in a side thread, reported the same way; a folder
// is trusted with -c projects={"<dir>"={trust_level="trusted"}}; a user's own
// hooks put a review screen in front of the session; a bracketed paste into
// the TUI arrives as one message.
func (r *CodexRunner) runTerminal(ctx context.Context, req RunRequest) (RunResult, error) {
	res, err := terminalRun(ctx, req, func(runDir string, sess *terminalSession) (string, func(), error) {
		sess.Expect = req.Prompt
		// The user's own hooks are theirs, not a worker's: continue without them.
		sess.Dismiss = []screenAnswer{{Match: "Hooks need review", Keys: []string{"3", "Enter"}}}

		notify := filepath.Join(runDir, "codex-notify.sh")
		body := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> " + shellQuote(sess.TurnsPath()) + "\n"
		if err := os.WriteFile(notify, []byte(body), 0o700); err != nil {
			return "", nil, err
		}

		args := []string{
			"-C", req.Cwd,
			"--sandbox", r.sandbox(),
			"--ask-for-approval", "on-request",
			"-c", "notify=" + tomlStrings("sh", notify),
			"-c", fmt.Sprintf("projects={%s={trust_level=\"trusted\"}}", tomlString(req.Cwd)),
		}
		if provider, model := splitModel(req.Model); provider == "codex" && model != "" && model != "default" {
			args = append(args, "-m", model)
		}

		env := baseTerminalEnv(append(append([]string{}, codexKeyEnv...), "CODEX_HOME", codexMCPTokenEnv))
		if r.Home != "" {
			env += "export CODEX_HOME=" + shellQuote(r.Home) + "\n"
		}
		if req.MCP != nil {
			args = append(args, codexMCPArgs(*req.MCP, codexMCPTokenEnv)...)
			env += "export " + codexMCPTokenEnv + "=" + shellQuote(req.MCP.Token) + "\n"
		}
		return terminalScript(runDir, env, req.Cwd, r.bin(), args), func() {}, nil
	})
	res.Billing = "subscription"
	return res, err
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// tomlStrings is a TOML array of strings.
func tomlStrings(ss ...string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = tomlString(s)
	}
	return "[" + strings.Join(q, ",") + "]"
}
