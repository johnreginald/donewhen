package mcp

import (
	"os"
	"strings"
	"testing"
)

// DW-109: the skill and the set_criteria text tell an agent how to choose a
// criterion kind: all four kinds and the example check of each.
func TestKindGuidanceNamesAllFourKindsWithExamples(t *testing.T) {
	files := map[string]string{
		"SKILL.md":     "../../skills/donewhen/SKILL.md",
		"TICKETS.md":   "../../skills/donewhen/TICKETS.md",
		"tools_dev.go": "tools_dev.go",
	}
	for name, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, want := range []string{"manual", "deterministic", "policy", "judgment", "paths_within", "expect_exit", "make test", "internal/**"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s does not mention %q", name, want)
			}
		}
	}
}
