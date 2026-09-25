package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// claudeModels are the aliases Claude Code resolves to its current models, so
// an agent set to "sonnet" moves with new releases rather than pinning one.
var claudeModels = []string{"default", "sonnet", "opus", "haiku"}

// CodexModels lists the models the Codex login offers, from the cache Codex
// keeps in its home, "default" first — the one config.toml names.
func CodexModels(home string) []string {
	if home == "" {
		if h := os.Getenv("CODEX_HOME"); h != "" {
			home = h
		} else if u, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(u, ".codex")
		}
	}
	out := []string{"default"}
	b, err := os.ReadFile(filepath.Join(home, "models_cache.json"))
	if err != nil {
		return out
	}
	var cache struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
			Priority   int    `json:"priority"`
		} `json:"models"`
	}
	if json.Unmarshal(b, &cache) != nil {
		return out
	}
	sort.SliceStable(cache.Models, func(i, j int) bool { return cache.Models[i].Priority < cache.Models[j].Priority })
	for _, m := range cache.Models {
		if m.Slug != "" && m.Visibility == "list" {
			out = append(out, m.Slug)
		}
	}
	return out
}

// OpenCodeProviders is what an OpenCode server can run: the providers it holds
// working credentials for, and their models as provider/model.
type OpenCodeProviders struct {
	Connected []string
	Models    []string
}

// Providers asks the OpenCode server which providers have credentials and
// what they offer. Raenil never sees the keys; OpenCode keeps them.
func (r *OpenCodeRunner) Providers(ctx context.Context) (OpenCodeProviders, error) {
	var resp struct {
		All []struct {
			ID     string                     `json:"id"`
			Models map[string]json.RawMessage `json:"models"`
		} `json:"all"`
		Connected []string `json:"connected"`
	}
	var out OpenCodeProviders
	if err := r.do(ctx, r.client(), http.MethodGet, "/provider", nil, nil, &resp); err != nil {
		return out, err
	}
	connected := map[string]bool{}
	for _, id := range resp.Connected {
		connected[id] = true
	}
	out.Connected = resp.Connected
	for _, p := range resp.All {
		if !connected[p.ID] {
			continue
		}
		var ids []string
		for m := range p.Models {
			ids = append(ids, p.ID+"/"+m)
		}
		sort.Strings(ids)
		out.Models = append(out.Models, ids...)
	}
	return out, nil
}
