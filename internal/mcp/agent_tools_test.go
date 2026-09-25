package mcp

import (
	"sort"
	"testing"
)

func TestAgentServerHasOnlyAgentTools(t *testing.T) {
	d := &deps{}
	full := buildServer(d).ListTools()
	agent := buildAgentServer(d).ListTools()

	var got []string
	for name := range agent {
		got = append(got, name)
	}
	sort.Strings(got)
	if len(got) != len(agentTools) {
		t.Errorf("agent server has %v, want exactly %d tools", got, len(agentTools))
	}
	for name := range agentTools {
		if _, ok := agent[name]; !ok {
			t.Errorf("agent tool %q is missing — renamed on the full server?", name)
		}
		if _, ok := full[name]; !ok {
			t.Errorf("agent tool %q does not exist on the full server", name)
		}
	}
	if len(full) <= len(agent) {
		t.Errorf("full server has %d tools, agent %d: nothing was narrowed", len(full), len(agent))
	}
}
