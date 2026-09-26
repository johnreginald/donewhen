package orchestrator

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// A terminal run has no output stream to read usage from, so it is read from
// the session file each harness keeps.

// claudeSessionUsage sums a Claude session transcript's usage. Each assistant
// message is written once per content block with the same usage, so messages
// are counted once by id.
func claudeSessionUsage(path string) (TokenUsage, bool) {
	var u TokenUsage
	f, err := os.Open(path)
	if path == "" || err != nil {
		return u, false
	}
	defer f.Close()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	found := false
	for sc.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				ID    string `json:"id"`
				Usage *struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Type != "assistant" || ev.Message.Usage == nil {
			continue
		}
		if ev.Message.ID != "" {
			if seen[ev.Message.ID] {
				continue
			}
			seen[ev.Message.ID] = true
		}
		m := ev.Message.Usage
		u.Input += m.InputTokens
		u.Output += m.OutputTokens
		u.CacheRead += m.CacheReadInputTokens
		u.CacheCreation += m.CacheCreationInputTokens
		found = true
	}
	return u, found
}

// codexSessionUsage reads the session's last running total from its rollout
// file under CODEX_HOME/sessions. Codex counts cached input inside input.
func codexSessionUsage(home, threadID string) (TokenUsage, bool) {
	var u TokenUsage
	if threadID == "" {
		return u, false
	}
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return u, false
		}
		home = filepath.Join(h, ".codex")
	}
	matches, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*"+threadID+".jsonl"))
	if len(matches) == 0 {
		return u, false
	}
	f, err := os.Open(matches[len(matches)-1])
	if err != nil {
		return u, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	found := false
	for sc.Scan() {
		line := sc.Bytes()
		if !strings.Contains(string(line), `"token_count"`) {
			continue
		}
		var ev struct {
			Payload struct {
				Type string `json:"type"`
				Info *struct {
					Total struct {
						InputTokens       int `json:"input_tokens"`
						CachedInputTokens int `json:"cached_input_tokens"`
						OutputTokens      int `json:"output_tokens"`
					} `json:"total_token_usage"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &ev) != nil || ev.Payload.Type != "token_count" || ev.Payload.Info == nil {
			continue
		}
		t := ev.Payload.Info.Total
		u = TokenUsage{Input: t.InputTokens - t.CachedInputTokens, CacheRead: t.CachedInputTokens, Output: t.OutputTokens}
		found = true
	}
	return u, found
}
