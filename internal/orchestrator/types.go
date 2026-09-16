// Package orchestrator turns a ticket's done-when criteria into a verdict backed
// by evidence. Nothing here asks a model whether the work is finished: a command
// exits zero or it does not, a diff satisfies a policy or it does not. Model
// judgment is carried as advisory signal only.
package orchestrator

import (
	"encoding/json"
	"fmt"
	"time"

	"raenil/internal/models"
)

// DeterministicCheck runs a command and compares its exit code.
// JSON: {"cmd":"npm run build","expect_exit":0,"timeout":"10m"}
type DeterministicCheck struct {
	Cmd string `json:"cmd"`
	// ExpectExit defaults to 0 when omitted.
	ExpectExit *int `json:"expect_exit,omitempty"`
	// Timeout is a Go duration string. Empty means DefaultCheckTimeout.
	Timeout string `json:"timeout,omitempty"`
}

// PolicyCheck names a rule evaluated against the diff a run produced.
// JSON: {"policy":"paths_within","args":["src/**"]}
type PolicyCheck struct {
	Policy string   `json:"policy"`
	Args   []string `json:"args,omitempty"`
}

// JudgmentCheck asks a model. Advisory only — see Verdict.AdvisoryFlagged.
// JSON: {"prompt":"conforms to ADR-0042?","model":"opus-5.1"}
type JudgmentCheck struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model,omitempty"`
}

// DefaultCheckTimeout bounds any deterministic check that does not set its own.
// Every check is bounded: an unbounded one wedges the daemon with no diagnostic.
const DefaultCheckTimeout = 15 * time.Minute

// Evidence is the record of one criterion being evaluated. It is the unit the
// whole design turns on: a criterion is ticked because an Evidence says so, never
// because an agent claimed completion.
type Evidence struct {
	CriterionIndex int    `json:"criterion_index"`
	CriterionText  string `json:"criterion_text"`
	Kind           string `json:"kind"`
	Pass           bool   `json:"pass"`

	// Cmd and Exit are set for deterministic checks.
	Cmd  string `json:"cmd,omitempty"`
	Exit int    `json:"exit,omitempty"`
	// Policy and Detail are set for policy checks; Detail names what failed.
	Policy string `json:"policy,omitempty"`
	Detail string `json:"detail,omitempty"`

	// OutputPath is relative to the attempt directory; OutputSHA256 lets a later
	// reader prove the log was not edited after the fact.
	OutputPath   string `json:"output_path,omitempty"`
	OutputSHA256 string `json:"output_sha256,omitempty"`

	// CostUSD is what evaluating this criterion cost. Non-zero only for judgment
	// criteria, which call a model.
	CostUSD    float64   `json:"cost_usd,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	Timestamp  time.Time `json:"ts"`
	// Err records why evaluation itself failed (timeout, unknown policy). An
	// Evidence with Err set is never a pass.
	Err string `json:"error,omitempty"`
}

// Status is the outcome of one attempt at a ticket.
type Status string

const (
	// StatusPassed means every gating criterion passed.
	StatusPassed Status = "passed"
	// StatusFailed means at least one gating criterion failed; retry is possible.
	StatusFailed Status = "failed"
	// StatusBlocked means the attempt could not be evaluated at all — the runner
	// died, the worktree was lost. Distinct from failed: retrying the same work
	// is pointless until the cause is fixed.
	StatusBlocked Status = "blocked"
)

// DiffStat summarises what an attempt changed.
type DiffStat struct {
	Files      int `json:"files"`
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

// Verdict is the ~20-line summary the judgment plane reads. Worker logs never
// enter an orchestrator's context; this does.
type Verdict struct {
	Ticket  string `json:"ticket"`
	Attempt int    `json:"attempt"`
	Status  Status `json:"status"`
	Runner  string `json:"runner,omitempty"`
	Model   string `json:"model,omitempty"`

	Passed []int `json:"passed"`
	Failed []int `json:"failed"`
	// AdvisoryFlagged lists judgment criteria a model was unhappy with. These
	// never move Status on their own.
	AdvisoryFlagged []int `json:"advisory_flagged,omitempty"`

	DiffStat  DiffStat `json:"diff_stat"`
	CostUSD   float64  `json:"cost_usd"`
	DurationS int64    `json:"duration_s"`
	// SessionID lets the next attempt continue this one instead of starting
	// cold, so a repair keeps the failed attempt in context.
	SessionID string `json:"session_id,omitempty"`
	// Questions is anything the worker asked. A daemon cannot answer, so these
	// are why a ticket bounces to a human rather than retrying forever.
	Questions []string `json:"questions,omitempty"`
	// Next is the recommended action: retry, escalate, review, bounce.
	Next string `json:"next,omitempty"`
	// Blocked explains a StatusBlocked verdict.
	Blocked string `json:"blocked,omitempty"`
}

// ParsedCriterion is a models.Criterion with its check_spec decoded.
type ParsedCriterion struct {
	Index int
	Text  string
	Kind  string

	Deterministic *DeterministicCheck
	Policy        *PolicyCheck
	Judgment      *JudgmentCheck
}

// Gating reports whether this criterion can block a ticket. Judgment criteria
// are advisory by construction, and manual criteria are a human's business.
func (c ParsedCriterion) Gating() bool {
	return c.Kind == models.CriterionDeterministic || c.Kind == models.CriterionPolicy
}

// ParseCriteria decodes a stored checklist into executable form. It fails loudly
// rather than silently skipping a criterion it cannot read — a criterion that
// quietly disappears is a gate that quietly stops gating.
func ParseCriteria(in []models.Criterion) ([]ParsedCriterion, error) {
	out := make([]ParsedCriterion, 0, len(in))
	for i, c := range in {
		p := ParsedCriterion{Index: i, Text: c.Body, Kind: c.Kind}
		if p.Kind == "" {
			p.Kind = models.CriterionManual
		}
		switch p.Kind {
		case models.CriterionManual:
			// nothing to decode
		case models.CriterionDeterministic:
			var d DeterministicCheck
			if err := json.Unmarshal(c.CheckSpec, &d); err != nil {
				return nil, fmt.Errorf("criterion %d (%q): bad deterministic check: %w", i+1, c.Body, err)
			}
			if d.Cmd == "" {
				return nil, fmt.Errorf("criterion %d (%q): deterministic check has no cmd", i+1, c.Body)
			}
			p.Deterministic = &d
		case models.CriterionPolicy:
			var pc PolicyCheck
			if err := json.Unmarshal(c.CheckSpec, &pc); err != nil {
				return nil, fmt.Errorf("criterion %d (%q): bad policy check: %w", i+1, c.Body, err)
			}
			if pc.Policy == "" {
				return nil, fmt.Errorf("criterion %d (%q): policy check has no policy name", i+1, c.Body)
			}
			p.Policy = &pc
		case models.CriterionJudgment:
			var j JudgmentCheck
			if err := json.Unmarshal(c.CheckSpec, &j); err != nil {
				return nil, fmt.Errorf("criterion %d (%q): bad judgment check: %w", i+1, c.Body, err)
			}
			p.Judgment = &j
		default:
			return nil, fmt.Errorf("criterion %d (%q): unknown kind %q", i+1, c.Body, p.Kind)
		}
		out = append(out, p)
	}
	return out, nil
}
