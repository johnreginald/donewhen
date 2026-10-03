package store

import (
	"context"
	"testing"
)

// DW-91: a new workspace has the standard label groups and no `runner` group.
func TestNewWorkspaceHasNoRunnerLabels(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsID := newWorkspace(t, s)

	groups, err := s.ListLabelGroups(ctx, wsID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, g := range groups {
		got[g.Name] = true
	}
	for _, want := range []string{"repo", "platform", "type", "domain", "triage"} {
		if !got[want] {
			t.Errorf("group %q is missing", want)
		}
	}
	if got["runner"] {
		t.Error("a new workspace must not get a runner group")
	}
	labels, err := s.ListLabels(ctx, wsID)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range labels {
		if l.Name == "codex" || l.Name == "opencode" {
			t.Errorf("a new workspace must not get the %q label", l.Name)
		}
	}
}
