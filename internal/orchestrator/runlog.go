package orchestrator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// maxRunLogTail is how much of a transcript leaves this machine with a run
// record. The full log stays in the run directory.
const maxRunLogTail = 32 * 1024

// redactPatterns match credentials by shape. A transcript carries tool output,
// and tool output carries whatever the agent printed — including keys.
var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{8,}`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{20,}`),
	regexp.MustCompile(`\braenil_[A-Za-z0-9_\-]{8,}`),
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`\bxox[abpr]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._\-]{16,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
}

// secretEnvName picks environment variables whose values must never appear in
// a transcript, whatever their shape.
var secretEnvName = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL|DSN|DATABASE_URL)`)

// redact removes credentials from text that is about to leave this machine.
func redact(s string) string {
	// Values first, longest first, so a value containing another is not left
	// half-replaced.
	var values []string
	for _, kv := range os.Environ() {
		name, val, ok := strings.Cut(kv, "=")
		if ok && len(val) >= 8 && secretEnvName.MatchString(name) {
			values = append(values, val)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, v := range values {
		s = strings.ReplaceAll(s, v, "[redacted]")
	}
	for _, re := range redactPatterns {
		if re.NumSubexp() > 0 {
			s = re.ReplaceAllString(s, "${1}[redacted]")
		} else {
			s = re.ReplaceAllString(s, "[redacted]")
		}
	}
	return s
}

// runLogTail returns the end of a run's log, readable and redacted. Claude's
// stream-json is rendered as a transcript; anything else is kept as written.
func runLogTail(path, runner string) string {
	var text string
	if runner == "claude" {
		text = claudeReadable(path)
	} else if b, err := os.ReadFile(path); err == nil {
		text = string(b)
	}
	if len(text) > maxRunLogTail {
		text = text[len(text)-maxRunLogTail:]
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:] // start on a whole line
		}
	}
	return redact(text)
}

// claudeReadable turns a stream-json log into lines a person can follow: what
// the agent said, each tool it called, and what came back.
func claudeReadable(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var b strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if line[0] != '{' {
			b.WriteString(string(line) + "\n") // the CLI's own stderr
			continue
		}
		var ev struct {
			Type      string  `json:"type"`
			Subtype   string  `json:"subtype"`
			Model     string  `json:"model"`
			NumTurns  int     `json:"num_turns"`
			Result    string  `json:"result"`
			IsError   bool    `json:"is_error"`
			CostUSD   float64 `json:"total_cost_usd"`
			SessionID string  `json:"session_id"`
			Message   struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "system":
			if ev.Subtype == "init" {
				fmt.Fprintf(&b, "· session %s · %s\n", ev.SessionID, ev.Model)
			}
		case "assistant":
			var parts []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			if json.Unmarshal(ev.Message.Content, &parts) != nil {
				continue
			}
			for _, p := range parts {
				switch p.Type {
				case "text":
					if t := strings.TrimSpace(p.Text); t != "" {
						b.WriteString(t + "\n")
					}
				case "tool_use":
					fmt.Fprintf(&b, "→ %s %s\n", p.Name, trunc(claudeToolInput(p.Input), 300))
				}
			}
		case "user":
			var parts []struct {
				Type    string          `json:"type"`
				Content json.RawMessage `json:"content"`
				IsError bool            `json:"is_error"`
			}
			if json.Unmarshal(ev.Message.Content, &parts) != nil {
				continue
			}
			for _, p := range parts {
				if p.Type != "tool_result" {
					continue
				}
				mark := "←"
				if p.IsError {
					mark = "✗"
				}
				fmt.Fprintf(&b, "%s %s\n", mark, trunc(oneLine(toolResultText(p.Content)), 300))
			}
		case "result":
			fmt.Fprintf(&b, "· %s · %d turns\n", ev.Subtype, ev.NumTurns)
			if ev.IsError && ev.Result != "" {
				b.WriteString(trunc(ev.Result, 500) + "\n")
			}
		}
	}
	return b.String()
}

// toolResultText reads a tool result's content, which is either a string or a
// list of text blocks.
func toolResultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			parts = append(parts, b.Text)
		}
		return strings.Join(parts, " ")
	}
	return string(raw)
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
