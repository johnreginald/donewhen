package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"raenil/internal/models"
)

// Label groups a ticket uses to say how it should be worked.
const (
	RunnerGroup = "runner"
	RepoGroup   = "repo"
)

// labelValue reads the value a ticket carries from one label group.
//
// Group membership is the convention this tracker already uses — the type group
// holds "bug", not "type:bug" — so that is matched first. A "group:value" name
// is also accepted and stripped, because both spellings exist in the wild and
// neither should quietly fail to match.
func labelValue(issue models.Issue, group, groupID string) (string, bool) {
	prefix := strings.ToLower(group) + ":"
	for _, l := range issue.Labels {
		name := strings.TrimSpace(l.Name)
		inGroup := groupID != "" && l.GroupID != nil && *l.GroupID == groupID
		hasPrefix := strings.HasPrefix(strings.ToLower(name), prefix)
		if !inGroup && !hasPrefix {
			continue
		}
		if hasPrefix {
			name = name[len(prefix):]
		}
		if v := strings.ToLower(strings.TrimSpace(name)); v != "" {
			return v, true
		}
	}
	return "", false
}

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
func (s RunnerSet) RunnerFor(issue models.Issue, groupID string, fallback Runner) (Runner, error) {
	if want, ok := labelValue(issue, RunnerGroup, groupID); ok {
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
func (s RepoSet) RepoFor(issue models.Issue, groupID string, fallback string) (string, error) {
	if want, ok := labelValue(issue, RepoGroup, groupID); ok {
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
