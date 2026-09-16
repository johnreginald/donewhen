package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"raenil/internal/models"
)

// RepoFacts is what a repository can tell us about how it is built and tested.
// A model asked to write verification commands without this invents plausible
// ones that do not exist, which is worse than no criteria at all.
type RepoFacts struct {
	Root        string
	Languages   []string
	Scripts     map[string]string // package.json scripts
	MakeTargets []string
	TopLevel    []string
	HasTests    bool
	Notes       []string
}

// InspectRepo reads the build and test surface of a repository.
func InspectRepo(root string) (RepoFacts, error) {
	f := RepoFacts{Root: root, Scripts: map[string]string{}}

	entries, err := os.ReadDir(root)
	if err != nil {
		return f, err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && name != ".github" {
			continue
		}
		f.TopLevel = append(f.TopLevel, name)
	}
	sort.Strings(f.TopLevel)

	if b, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		f.Languages = append(f.Languages, "node")
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(b, &pkg) == nil {
			f.Scripts = pkg.Scripts
		}
		if _, ok := f.Scripts["test"]; ok {
			f.HasTests = true
		} else {
			f.Notes = append(f.Notes, "package.json has no test script — do not invent one")
		}
		for _, lock := range []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock"} {
			if _, err := os.Stat(filepath.Join(root, lock)); err == nil {
				f.Notes = append(f.Notes, "lockfile is "+lock)
				break
			}
		}
		f.Notes = append(f.Notes,
			"a worktree is a clean checkout with no node_modules, so any command needing "+
				"dependencies must install them first (e.g. `npm ci && npm run build`)")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		f.Languages = append(f.Languages, "go")
		f.HasTests = true
		f.Notes = append(f.Notes, "go test ./... works without a separate install step")
	}
	for _, py := range []string{"pyproject.toml", "requirements.txt", "setup.py"} {
		if _, err := os.Stat(filepath.Join(root, py)); err == nil {
			f.Languages = append(f.Languages, "python")
			break
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "Makefile")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if i := strings.Index(line, ":"); i > 0 && !strings.HasPrefix(line, "\t") &&
				!strings.HasPrefix(line, "#") && !strings.Contains(line[:i], "=") {
				target := strings.TrimSpace(line[:i])
				if target != "" && !strings.Contains(target, " ") && !strings.HasPrefix(target, ".") {
					f.MakeTargets = append(f.MakeTargets, target)
				}
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err != nil {
		f.Notes = append(f.Notes,
			"this repository has NO .gitignore — build artifacts will land in the diff "+
				"and trip path policies")
	}
	return f, nil
}

// String renders the facts for a prompt.
func (f RepoFacts) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Languages: %s\n", strings.Join(orNone(f.Languages), ", "))
	fmt.Fprintf(&b, "Top level: %s\n", strings.Join(f.TopLevel, ", "))
	if len(f.Scripts) > 0 {
		b.WriteString("package.json scripts:\n")
		keys := make([]string, 0, len(f.Scripts))
		for k := range f.Scripts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  %s = %s\n", k, f.Scripts[k])
		}
	}
	if len(f.MakeTargets) > 0 {
		fmt.Fprintf(&b, "Make targets: %s\n", strings.Join(f.MakeTargets, ", "))
	}
	for _, n := range f.Notes {
		fmt.Fprintf(&b, "NOTE: %s\n", n)
	}
	return b.String()
}

func orNone(s []string) []string {
	if len(s) == 0 {
		return []string{"unknown"}
	}
	return s
}

// ProposedCriterion is one drafted checklist item.
type ProposedCriterion struct {
	Text  string          `json:"text"`
	Kind  string          `json:"kind"`
	Check json.RawMessage `json:"check,omitempty"`
}

// Propose drafts a typed checklist for an issue.
//
// The model writes the draft; it does not get to approve it. Everything it
// returns is parsed through the same ParseCriteria the orchestrator uses, so a
// criterion that would not have gated anything never reaches the ticket.
func Propose(ctx context.Context, a Asker, model string, issue models.Issue, facts RepoFacts) ([]ProposedCriterion, error) {
	if model == "" {
		return nil, fmt.Errorf("a model is required to draft criteria")
	}

	// Models occasionally return a truncated or prose-wrapped reply. One retry
	// with a blunter instruction costs a cent and saves a manual re-run.
	prompt := proposePrompt(issue, facts)
	var out []ProposedCriterion
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			prompt = proposePrompt(issue, facts) +
				"\nYour previous reply could not be parsed. Reply with the raw JSON array only: " +
				"no prose, no code fence, no trailing commentary.\n"
		}
		answer, err := a.Ask(ctx, model, prompt)
		if err != nil {
			return nil, err
		}
		raw, err := extractJSONArray(answer)
		if err != nil {
			lastErr = fmt.Errorf("%w\n--- model said ---\n%s", err, trunc(answer, 600))
			continue
		}
		out = nil
		if err := json.Unmarshal(raw, &out); err != nil {
			lastErr = fmt.Errorf("draft is not a criteria array: %w\n--- extracted ---\n%s\n--- model said ---\n%s",
				err, trunc(string(raw), 400), trunc(answer, 800))
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("model proposed no criteria")
	}

	// Validate through the real parser. A draft that cannot gate is not a draft.
	if _, err := ParseCriteria(proposedToModels(out)); err != nil {
		return nil, fmt.Errorf("drafted criteria are unusable: %w", err)
	}
	parsed, _ := ParseCriteria(proposedToModels(out))
	if !anyGating(parsed) {
		return nil, fmt.Errorf("drafted criteria contain nothing machine-checkable")
	}
	return out, nil
}

func proposedToModels(in []ProposedCriterion) []models.Criterion {
	out := make([]models.Criterion, 0, len(in))
	for _, c := range in {
		out = append(out, models.Criterion{Body: c.Text, Kind: c.Kind, CheckSpec: c.Check})
	}
	return out
}

func proposePrompt(issue models.Issue, facts RepoFacts) string {
	var b strings.Builder
	b.WriteString("Write the done-when acceptance checklist for the ticket below.\n\n")
	b.WriteString("An orchestrator will run these checks mechanically to decide whether an " +
		"agent's work is acceptable. Nothing the agent claims counts; only these do.\n\n")

	fmt.Fprintf(&b, "## Ticket %s: %s\n\n%s\n\n", issue.Key, issue.Title, issue.DescriptionMD)
	fmt.Fprintf(&b, "## The repository\n\n%s\n", facts)

	b.WriteString(`## Output

Reply with a JSON array and nothing else. Each item is:

  {"text": "...", "kind": "deterministic", "check": {"cmd": "...", "expect_exit": 0, "timeout": "5m"}}
  {"text": "...", "kind": "policy", "check": {"policy": "paths_within", "args": ["src/**"]}}
  {"text": "...", "kind": "judgment", "check": {"prompt": "..."}}
  {"text": "...", "kind": "manual"}

Policies available: paths_within, paths_forbidden, no_new_deps, no_secrets,
max_diff_lines, tests_not_weakened. Globs take * within a segment and ** across.

## Rules

- Commands run in a clean git worktree of this repository, from its root.
- Only use commands that exist here. Do not invent a test runner or a script
  that is not listed above. If there is no test setup, do not pretend there is.
- Every check must FAIL on the repository as it stands today and PASS once the
  ticket is done. A check that already passes gates nothing.
- Include at least one deterministic check that directly verifies the ticket's
  actual outcome, not just that the project still builds.
- Include a paths_within policy scoping the work, and tests_not_weakened
  whenever the repository has tests.
- Prefer three to six criteria. Each must be independently checkable.
`)
	return b.String()
}

// extractJSONArray pulls a JSON array out of a model's reply, which often
// arrives wrapped in prose or a code fence.
func extractJSONArray(s string) (json.RawMessage, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		if j := strings.IndexByte(rest, '\n'); j >= 0 {
			rest = rest[j+1:]
		}
		if k := strings.Index(rest, "```"); k >= 0 {
			s = strings.TrimSpace(rest[:k])
		}
	}
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON array in the reply")
	}
	return json.RawMessage(s[start : end+1]), nil
}

// VerifyCriteriaFailToday runs the drafted deterministic checks against the
// repository as it stands. Every one should fail: a check that already passes
// gates nothing, and a checklist full of those would wave any work through.
func VerifyCriteriaFailToday(ctx context.Context, repo string, items []ProposedCriterion) ([]string, error) {
	parsed, err := ParseCriteria(proposedToModels(items))
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "raenil-verify-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	ev := &Evaluator{WorkDir: repo, Dir: nil}
	var out []string
	for _, c := range parsed {
		if c.Kind != models.CriterionDeterministic {
			out = append(out, fmt.Sprintf("%d. [%s] not run here", c.Index+1, c.Kind))
			continue
		}
		logPath := filepath.Join(dir, fmt.Sprintf("verify-%d.log", c.Index))
		exit, runErr := ev.runCommand(ctx, *c.Deterministic, logPath)
		want := 0
		if c.Deterministic.ExpectExit != nil {
			want = *c.Deterministic.ExpectExit
		}
		switch {
		case runErr != nil:
			out = append(out, fmt.Sprintf("%d. could not run (%v) — fix the command", c.Index+1, runErr))
		case exit == want:
			out = append(out, fmt.Sprintf("%d. ALREADY PASSES — this gates nothing: %s", c.Index+1, c.Text))
		default:
			out = append(out, fmt.Sprintf("%d. fails today (exit %d), as it should", c.Index+1, exit))
		}
	}
	return out, nil
}
