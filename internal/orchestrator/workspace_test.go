package orchestrator

import "testing"

func TestSplitIssueKey(t *testing.T) {
	cases := []struct {
		in, prefix string
		ok         bool
	}{
		{"PP-175", "PP", true},
		{"API-42", "API", true},
		{"api-1", "api", true},
		{"PP", "", false},
		{"PP-", "", false},
		{"PP-abc", "", false},
		{"", "", false},
		// A UUID must not be mistaken for a key.
		{"c577298b-ac78-4a8d-9c46-eac4072b63ea", "", false},
	}
	for _, c := range cases {
		p, _, ok := splitIssueKey(c.in)
		if ok != c.ok || (ok && p != c.prefix) {
			t.Errorf("splitIssueKey(%q) = %q,%v want %q,%v", c.in, p, ok, c.prefix, c.ok)
		}
	}
}
