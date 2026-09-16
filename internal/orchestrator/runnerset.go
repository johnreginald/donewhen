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

// RepoLabelPrefix is the label group that says which repository a ticket is
// worked in, following the repo's exclusive-group convention: repo:api-mobile.
const RepoLabelPrefix = "repo:"

// RepoSet maps a repo: label value to a checkout on this machine.
type RepoSet map[string]string

// Names lists the configured repositories, sorted, for error messages.
func (s RepoSet) Names() []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RepoFor picks the checkout a ticket names.
//
// A ticket naming a repository this machine has no path for is an error, not a
// fallback. A project with several repositories is exactly where a silent
// default does the most damage: the work would be written, committed and
// verified in the wrong codebase, and every check would pass.
func (s RepoSet) RepoFor(issue models.Issue, fallback string) (string, error) {
	for _, l := range issue.Labels {
		name := strings.TrimSpace(l.Name)
		if !strings.HasPrefix(strings.ToLower(name), RepoLabelPrefix) {
			continue
		}
		want := strings.ToLower(strings.TrimSpace(name[len(RepoLabelPrefix):]))
		if path, ok := s[want]; ok {
			return path, nil
		}
		return "", fmt.Errorf("issue %s asks for repository %q, which is not configured (have: %s)",
			issue.Key, want, strings.Join(s.Names(), ", "))
	}
	if fallback == "" {
		return "", fmt.Errorf("issue %s has no repo: label and no --repo was given", issue.Key)
	}
	return fallback, nil
}
