package orchestrator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Every harness's transcript is posted in one readable shape, so the ticket's
// conversation can show any agent the same way:
//
//	· meta          (session, model, how it ended)
//	→ tool input    (a call the agent made)
//	← result        (what came back)
//	✗ error         (a call that failed)
//	anything else   (what the agent said)
//
// claudeLine renders Claude's stream-json; codexLine and opencodeReadable
// render Codex's JSONL events and OpenCode's session dump into the same lines.

// codexLine renders one line of `codex exec --json` output. Lines that are not
// JSON are Codex's own diagnostics, kept as meta so they never read as the
// agent speaking.
func codexLine(line []byte) []string {
	s := strings.TrimSpace(string(line))
	if s == "" {
		return nil
	}
	if s[0] != '{' {
		return []string{"· " + trunc(s, 300)}
	}
	var ev struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
		Message  string `json:"message"`
		Error    struct {
			Message string `json:"message"`
		} `json:"error"`
		Item map[string]json.RawMessage `json:"item"`
	}
	if json.Unmarshal(line, &ev) != nil {
		return nil
	}
	switch ev.Type {
	case "thread.started":
		return []string{"· session " + ev.ThreadID + " · codex"}
	case "turn.failed":
		return []string{"✗ " + trunc(ev.Error.Message, 500)}
	case "error":
		return []string{"· " + trunc(ev.Message, 300)}
	case "item.completed":
		return codexItem(ev.Item)
	}
	return nil
}

func codexItem(item map[string]json.RawMessage) []string {
	str := func(k string) string {
		var v string
		_ = json.Unmarshal(item[k], &v)
		return v
	}
	switch typ := str("type"); typ {
	case "agent_message":
		if t := strings.TrimSpace(str("text")); t != "" {
			return []string{t}
		}
	case "reasoning":
		return nil // the model thinking to itself; the answer carries what matters
	case "command_execution":
		out := []string{"→ shell " + trunc(str("command"), 300)}
		var code *int
		_ = json.Unmarshal(item["exit_code"], &code)
		mark := "←"
		if code != nil && *code != 0 {
			mark = "✗"
		}
		if res := oneLine(str("aggregated_output")); res != "" || mark == "✗" {
			out = append(out, fmt.Sprintf("%s %s", mark, trunc(res, 300)))
		}
		return out
	case "file_change":
		var changes []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(item["changes"], &changes)
		var out []string
		for _, c := range changes {
			out = append(out, fmt.Sprintf("→ %s %s", codexChangeVerb(c.Kind), c.Path))
		}
		return out
	case "mcp_tool_call":
		out := []string{fmt.Sprintf("→ %s.%s %s", str("server"), str("tool"), trunc(oneLine(string(item["arguments"])), 300))}
		if e := str("error"); e != "" {
			out = append(out, "✗ "+trunc(oneLine(e), 300))
		}
		return out
	case "web_search":
		return []string{"→ web_search " + trunc(str("query"), 300)}
	case "error":
		return []string{"· " + trunc(str("message"), 300)}
	case "todo_list":
		return nil
	default:
		if typ != "" {
			return []string{"→ " + typ}
		}
	}
	return nil
}

func codexChangeVerb(kind string) string {
	switch kind {
	case "add":
		return "create"
	case "delete":
		return "delete"
	}
	return "edit"
}

// opencodeReadable renders the session OpenCode's runner saves at the end of a
// run: the assistant's text, each tool call with its result. The first user
// message is Raenil's own prompt and is left out.
func opencodeReadable(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var msgs []struct {
		Info struct {
			Role      string `json:"role"`
			SessionID string `json:"sessionID"`
			ModelID   string `json:"modelID"`
		} `json:"info"`
		Parts []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Tool  string `json:"tool"`
			State struct {
				Status string          `json:"status"`
				Input  json.RawMessage `json:"input"`
				Output string          `json:"output"`
				Error  string          `json:"error"`
			} `json:"state"`
		} `json:"parts"`
	}
	if json.Unmarshal(b, &msgs) != nil {
		return nil
	}
	var out []string
	for i, m := range msgs {
		if i == 0 && m.Info.SessionID != "" {
			out = append(out, "· session "+m.Info.SessionID+" · opencode")
		}
		if m.Info.Role != "assistant" {
			continue
		}
		for _, p := range m.Parts {
			switch p.Type {
			case "text":
				if t := strings.TrimSpace(p.Text); t != "" {
					out = append(out, t)
				}
			case "tool":
				out = append(out, fmt.Sprintf("→ %s %s", p.Tool, trunc(claudeToolInput(p.State.Input), 300)))
				switch {
				case p.State.Status == "error" || p.State.Error != "":
					out = append(out, "✗ "+trunc(oneLine(p.State.Error), 300))
				case strings.TrimSpace(p.State.Output) != "":
					out = append(out, "← "+trunc(oneLine(p.State.Output), 300))
				}
			}
		}
	}
	return out
}

// readableLines renders a whole run log for its runner.
func readableLines(path, runner string) []string {
	switch runner {
	case "opencode":
		return opencodeReadable(path)
	case "claude", "codex", "antigravity":
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		render := claudeLine
		switch runner {
		case "codex":
			render = codexLine
		case "antigravity":
			render = agyLine
		}
		var out []string
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
		for sc.Scan() {
			out = append(out, render(sc.Bytes())...)
		}
		return out
	}
	return nil
}

// agyLine renders one Antigravity stream-json event: each finished tool call
// and the final answer. Streamed text deltas are left for the final answer.
func agyLine(line []byte) []string {
	var ev struct {
		Event string `json:"event"`
		Step  struct {
			State    string `json:"state"`
			StepType string `json:"step_type"`
			ToolName string `json:"tool_name"`
			ToolInfo struct {
				Parameters map[string]any `json:"parameters"`
			} `json:"tool_info"`
		} `json:"step_update"`
		Result struct {
			Response string `json:"response"`
		} `json:"result"`
	}
	if json.Unmarshal(line, &ev) != nil {
		return nil
	}
	switch {
	case ev.Event == "result" && strings.TrimSpace(ev.Result.Response) != "":
		return []string{strings.TrimSpace(ev.Result.Response)}
	case ev.Event == "step_update" && ev.Step.StepType == "tool" && ev.Step.State == "DONE":
		arg := ""
		for _, k := range []string{"CommandLine", "TargetFile", "AbsolutePath", "SearchPath", "Query", "DirectoryPath"} {
			if v, ok := ev.Step.ToolInfo.Parameters[k].(string); ok && v != "" {
				arg = v
				break
			}
		}
		return []string{"→ " + ev.Step.ToolName + " " + trunc(oneLine(arg), 200)}
	}
	return nil
}
