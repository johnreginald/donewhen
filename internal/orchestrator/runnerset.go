package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"raenil/internal/models"
)

// RunnerLabelPrefix is the label group that picks a ticket's worker, following
// the repo's exclusive-group convention: runner:codex, runner:opencode.
const RunnerLabelPrefix = "runner:"

// RunnerSet is the pool a ticket can choose from, keyed by runner name.
type RunnerSet map[string]Runner

// Names lists the available runners, sorted, for error messages.
func (s RunnerSet) Names() []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RunnerFor picks the runner a ticket asked for.
//
// A ticket that names a runner it cannot have is an error rather than a silent
// fallback: someone labelled it deliberately, and quietly running the work on a
// different agent than the one requested is the kind of substitution nobody
// catches until the results look strange.
func (s RunnerSet) RunnerFor(issue models.Issue, fallback Runner) (Runner, error) {
	for _, l := range issue.Labels {
		name := strings.TrimSpace(l.Name)
		if !strings.HasPrefix(strings.ToLower(name), RunnerLabelPrefix) {
			continue
		}
		want := strings.ToLower(strings.TrimSpace(name[len(RunnerLabelPrefix):]))
		if r, ok := s[want]; ok {
			return r, nil
		}
		return nil, fmt.Errorf("issue %s asks for runner %q, which is not configured (have: %s)",
			issue.Key, want, strings.Join(s.Names(), ", "))
	}
	if fallback == nil {
		return nil, fmt.Errorf("no runner configured")
	}
	return fallback, nil
}
