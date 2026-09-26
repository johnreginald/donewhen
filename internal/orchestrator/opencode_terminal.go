package orchestrator

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// CanRunInTerminal implements terminalRunner.
func (r *OpenCodeRunner) CanRunInTerminal() bool { return true }

// runTerminal works a ticket on the OpenCode server as a headless run does,
// with the TUI attached to the same session in a tmux session so the user can
// watch and type into it. Raenil still sends the prompt and the check
// failures through the server, and tool permissions are still answered by the
// runner's policy; the agent's questions are left for the user in the TUI.
func (r *OpenCodeRunner) runTerminal(ctx context.Context, req RunRequest) (RunResult, error) {
	dirQ := url.Values{"directory": {req.Cwd}}
	var sessionID string
	res, err := terminalRun(ctx, req, func(runDir string, sess *terminalSession) (string, func(), error) {
		sessionID = req.SessionID
		if sessionID == "" {
			var s ocSession
			if err := r.do(ctx, r.client(), http.MethodPost, "/session", dirQ,
				map[string]any{"title": req.Title}, &s); err != nil {
				return "", nil, fmt.Errorf("create session: %w", err)
			}
			sessionID = s.ID
		}
		cleanup := func() {}
		if req.MCP != nil {
			unlock := lockDir(req.Cwd)
			if err := r.addMCP(ctx, dirQ, *req.MCP); err != nil {
				unlock()
				return "", nil, fmt.Errorf("give the run Raenil's tools: %w", err)
			}
			cleanup = func() {
				_ = r.do(context.WithoutCancel(ctx), r.client(), http.MethodPost,
					"/mcp/"+req.MCP.Name+"/disconnect", dirQ, nil, nil)
				unlock()
			}
		}

		send := func(ctx context.Context, text string) error {
			prompt := map[string]any{"parts": []map[string]any{{"type": "text", "text": text}}}
			if provider, model := splitModel(req.Model); provider != "" && model != "" {
				prompt["model"] = map[string]string{"providerID": provider, "modelID": model}
			}
			if r.Agent != "" {
				prompt["agent"] = r.Agent
			}
			return r.do(ctx, r.client(), http.MethodPost, "/session/"+sessionID+"/prompt_async", dirQ, prompt, nil)
		}
		sess.SendFn = send
		sess.WaitFn = r.terminalWait(sessionID, req)
		if err := send(ctx, req.Prompt); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("dispatch prompt: %w", err)
		}

		env := baseTerminalEnv(nil)
		envPath := filepath.Join(runDir, "terminal.env")
		if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
			cleanup()
			return "", nil, err
		}
		script := "#!/bin/sh\n. " + shellQuote(envPath) + "\nrm -f " + shellQuote(envPath) + "\n" +
			"cd " + shellQuote(req.Cwd) + " || exit 1\n" +
			"exec opencode attach " + shellQuote(r.BaseURL) + " --dir " + shellQuote(req.Cwd) +
			" --session " + shellQuote(sessionID) + "\n"
		return script, cleanup, nil
	})
	res.Billing = "api"
	res.SessionID = sessionID
	if sessionID != "" {
		tail, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if res.Aborted || ctx.Err() != nil {
			_ = r.do(tail, r.client(), http.MethodPost, "/session/"+sessionID+"/abort", dirQ, nil, nil)
		}
		var s ocSession
		if err := r.do(tail, r.client(), http.MethodGet, "/session/"+sessionID, dirQ, nil, &s); err == nil {
			res.CostUSD, res.CostUnknown = s.Cost, false
			res.Tokens = int(s.Tokens.Input + s.Tokens.Output + s.Tokens.Reasoning)
		}
		if req.LogPath != "" {
			r.dumpTranscript(tail, sessionID, req.Cwd, filepath.Join(filepath.Dir(req.LogPath), "opencode-transcript.json"))
		}
	}
	return res, err
}

// terminalWait is the turn signal for a terminal OpenCode session: the
// session's last message is an assistant message that has completed and not
// been reported yet. Tool permissions are answered as a headless run answers
// them; a question the agent asks is left for the user, who is told once.
func (r *OpenCodeRunner) terminalWait(sessionID string, req RunRequest) func(context.Context) (Turn, error) {
	q := url.Values{"directory": {req.Cwd}}
	reported := ""
	asked := map[string]bool{}
	policy := r.Permission
	if policy == nil {
		policy = ApproveOncePolicy
	}
	return func(ctx context.Context) (Turn, error) {
		tick := time.NewTicker(r.pollInterval())
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return Turn{}, ctx.Err()
			case <-tick.C:
			}
			var perms []PermissionRequest
			if err := r.do(ctx, r.client(), http.MethodGet, "/permission", nil, nil, &perms); err == nil {
				for _, p := range perms {
					if p.SessionID != sessionID {
						continue
					}
					decision, reason := policy(p)
					body := map[string]any{"reply": string(decision)}
					if reason != "" {
						body["message"] = reason
					}
					_ = r.do(ctx, r.client(), http.MethodPost, "/permission/"+p.ID+"/reply", nil, body, nil)
				}
			}
			var questions []QuestionRequest
			if err := r.do(ctx, r.client(), http.MethodGet, "/question", nil, nil, &questions); err == nil {
				for _, qu := range questions {
					if qu.SessionID == sessionID && !asked[qu.ID] {
						asked[qu.ID] = true
						if req.OnQuestion != nil {
							req.OnQuestion(qu.Text)
						}
					}
				}
			}
			var msgs []ocMessage
			if err := r.do(ctx, r.client(), http.MethodGet, "/session/"+sessionID+"/message", q, nil, &msgs); err != nil || len(msgs) == 0 {
				continue
			}
			last := msgs[len(msgs)-1]
			if last.Info.Role != "assistant" || last.Info.Time.Completed == 0 || last.Info.ID == reported {
				continue
			}
			reported = last.Info.ID
			return Turn{SessionID: sessionID, Message: lastAssistantText(msgs)}, nil
		}
	}
}
