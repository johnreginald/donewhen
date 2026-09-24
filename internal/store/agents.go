package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

const agentCols = `id, name, slug, role, harness, model, effort, instructions_md, allowed_tools,
	max_turns, status, created_at, updated_at`

func scanAgent(row pgx.Row) (models.Agent, error) {
	var a models.Agent
	var allowed []byte
	err := row.Scan(&a.ID, &a.Name, &a.Slug, &a.Role, &a.Harness, &a.Model, &a.Effort, &a.InstructionsMD,
		&allowed, &a.MaxTurns, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	if json.Unmarshal(allowed, &a.AllowedTools) != nil || a.AllowedTools == nil {
		a.AllowedTools = []string{}
	}
	return a, nil
}

// AgentInput creates or changes an agent. Nil fields are left as they are.
type AgentInput struct {
	Name           *string
	Role           *string
	Harness        *string
	Model          *string
	Effort         *string
	InstructionsMD *string
	AllowedTools   []string
	SetAllowed     bool
	MaxTurns       *int
	Status         *string
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// agentSlug turns a name into the handle an agent is addressed by.
func agentSlug(name string) string {
	s := strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		s = "agent"
	}
	return s
}

func validateAgent(in AgentInput) error {
	if in.Harness != nil && !slices.Contains(models.Harnesses, *in.Harness) {
		return invalid("harness must be one of %s, not %q", strings.Join(models.Harnesses, ", "), *in.Harness)
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "paused" {
		return invalid("status must be active or paused, not %q", *in.Status)
	}
	if in.MaxTurns != nil && *in.MaxTurns < 0 {
		return invalid("max turns cannot be negative")
	}
	for _, rule := range in.AllowedTools {
		// Bare Bash pre-approves every shell command, writes anywhere
		// included — the live test watched a worker escape its worktree that
		// way. A rule has to name what it allows.
		if strings.TrimSpace(rule) == "Bash" || strings.TrimSpace(rule) == "Bash(*)" {
			return invalid("%q would allow any shell command; name the commands, e.g. Bash(go test *)", rule)
		}
	}
	return nil
}

// CreateAgent adds an agent to a workspace. Its slug comes from its name and
// is made unique within the workspace.
func (s *Store) CreateAgent(ctx context.Context, wsID string, in AgentInput) (models.Agent, error) {
	if in.Name == nil || strings.TrimSpace(*in.Name) == "" {
		return models.Agent{}, invalid("name is required")
	}
	if in.Harness == nil {
		return models.Agent{}, invalid("harness is required")
	}
	if err := validateAgent(in); err != nil {
		return models.Agent{}, err
	}
	name := strings.TrimSpace(*in.Name)
	allowed, _ := json.Marshal(nonNil(in.AllowedTools))
	base := agentSlug(name)
	for n := 1; n <= 20; n++ {
		slug := base
		if n > 1 {
			slug = fmt.Sprintf("%s-%d", base, n)
		}
		a, err := scanAgent(s.pool.QueryRow(ctx, `
			INSERT INTO agents (workspace_id, name, slug, role, harness, model, effort, instructions_md, allowed_tools, max_turns)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING `+agentCols,
			wsID, name, slug, deref(in.Role), *in.Harness, deref(in.Model), deref(in.Effort),
			deref(in.InstructionsMD), allowed, derefInt(in.MaxTurns)))
		if err == nil {
			return a, nil
		}
		if !isUniqueViolation(err, "") {
			return a, err
		}
	}
	return models.Agent{}, fmt.Errorf("could not find a free handle for %q: %w", name, ErrConflict)
}

// UpdateAgent changes an agent in this workspace.
func (s *Store) UpdateAgent(ctx context.Context, wsID, id string, in AgentInput) (models.Agent, error) {
	if err := validateAgent(in); err != nil {
		return models.Agent{}, err
	}
	sets, args := []string{}, []any{wsID, id}
	set := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return models.Agent{}, invalid("name cannot be empty")
		}
		set("name", strings.TrimSpace(*in.Name))
	}
	if in.Role != nil {
		set("role", *in.Role)
	}
	if in.Harness != nil {
		set("harness", *in.Harness)
	}
	if in.Model != nil {
		set("model", *in.Model)
	}
	if in.Effort != nil {
		set("effort", *in.Effort)
	}
	if in.InstructionsMD != nil {
		set("instructions_md", *in.InstructionsMD)
	}
	if in.SetAllowed {
		b, _ := json.Marshal(nonNil(in.AllowedTools))
		set("allowed_tools", b)
	}
	if in.MaxTurns != nil {
		set("max_turns", *in.MaxTurns)
	}
	if in.Status != nil {
		set("status", *in.Status)
	}
	if len(sets) == 0 {
		return s.GetAgent(ctx, wsID, id)
	}
	a, err := scanAgent(s.pool.QueryRow(ctx,
		`UPDATE agents SET `+strings.Join(sets, ", ")+`, updated_at = now()
		 WHERE workspace_id = $1 AND (id::text = $2 OR slug = $2) RETURNING `+agentCols, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// GetAgent reads an agent by id or slug.
func (s *Store) GetAgent(ctx context.Context, wsID, ref string) (models.Agent, error) {
	a, err := scanAgent(s.pool.QueryRow(ctx,
		`SELECT `+agentCols+` FROM agents WHERE workspace_id = $1 AND (id::text = $2 OR slug = $2)`, wsID, ref))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// ListAgents lists a workspace's agents by name.
func (s *Store) ListAgents(ctx context.Context, wsID string) ([]models.Agent, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+agentCols+` FROM agents WHERE workspace_id = $1 ORDER BY name`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAgent removes an agent. Tickets and runs keep their history; their
// agent reference is cleared.
func (s *Store) DeleteAgent(ctx context.Context, wsID, ref string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE workspace_id = $1 AND (id::text = $2 OR slug = $2)`, wsID, ref)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// checkAgentTx refuses an agent that is not in this workspace. The foreign key
// alone would let a ticket point at another workspace's agent.
func (s *Store) checkAgentTx(ctx context.Context, tx pgx.Tx, wsID string, agentID *string) error {
	if agentID == nil || *agentID == "" {
		return nil
	}
	var ok bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM agents WHERE id::text = $1 AND workspace_id = $2)`, *agentID, wsID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("agent %s: %w", *agentID, ErrNotFound)
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
