package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/johnreginald/donewhen/internal/models"
)

// ---- dev links (branch / PR) ----

// SetIssueDev sets the branch and/or the PR that implemented an issue. A nil
// argument leaves that column alone; an empty string clears it. Both nil is an
// error: there is nothing to set.
func (s *Store) SetIssueDev(ctx context.Context, wsID, issueID string, branch, prURL *string) (models.Issue, error) {
	if branch == nil && prURL == nil {
		return models.Issue{}, invalid("nothing to set: send gitBranch and/or prUrl")
	}
	setPR := prURL != nil
	prURL, err := normURL("prUrl", prURL)
	if err != nil {
		return models.Issue{}, err
	}
	setBranch := branch != nil
	if setBranch && *branch == "" {
		branch = nil
	}
	ct, err := s.pool.Exec(ctx,
		`UPDATE issues SET
			git_branch = CASE WHEN $5 THEN $3 ELSE git_branch END,
			pr_url     = CASE WHEN $6 THEN $4 ELSE pr_url END,
			updated_at = now()
		 WHERE id=$1 AND workspace_id=$2`,
		issueID, wsID, branch, prURL, setBranch, setPR)
	if err != nil {
		return models.Issue{}, err
	}
	if ct.RowsAffected() == 0 {
		return models.Issue{}, ErrNotFound
	}
	return s.GetIssue(ctx, wsID, issueID)
}

// normURL validates a link field (PR, commit, repo). Only absolute http(s) URLs
// are stored — they are rendered as hrefs, so anything else is a stored-XSS
// vector. Empty clears (returns nil).
func normURL(field string, raw *string) (*string, error) {
	v, err := models.NormalizeURL(raw)
	if err != nil {
		return nil, invalid("invalid_url: %s must be an absolute http or https URL", field)
	}
	return v, nil
}

// IssueRepo returns the default repo for an issue — its Epic's repo_url, else
// its Project's (initiative's) repo_url, else "".
func (s *Store) IssueRepo(ctx context.Context, wsID, issueID string) string {
	var repo *string
	err := s.pool.QueryRow(ctx, `
		SELECT coalesce(p.repo_url, i.repo_url)
		FROM issues iss
		LEFT JOIN projects p ON p.id = iss.project_id AND p.workspace_id = iss.workspace_id
		LEFT JOIN initiatives i ON i.id = p.initiative_id AND i.workspace_id = iss.workspace_id
		WHERE iss.id = $1 AND iss.workspace_id = $2`, issueID, wsID).Scan(&repo)
	if err != nil || repo == nil {
		return ""
	}
	return *repo
}

// commitURL builds a GitHub-style commit link from a repo URL + sha.
func commitURL(repo, sha string) string {
	if repo == "" || sha == "" {
		return ""
	}
	repo = strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
	return repo + "/commit/" + sha
}

// ---- commits ----

// NormalizeSHA checks a commit sha and returns it lowercase: 7 to 40 hex
// characters, nothing else. A prefix match against a stored sha is only
// meaningful for a real sha, so a malformed one is an error, not an empty result.
func NormalizeSHA(raw string) (string, error) {
	sha := strings.ToLower(strings.TrimSpace(raw))
	if sha == "" {
		return "", invalid("sha required")
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", invalid("sha must be hex")
		}
	}
	if len(sha) < 7 {
		return "", invalid("sha must be at least 7 characters")
	}
	if len(sha) > 40 {
		return "", invalid("sha must be at most 40 characters")
	}
	return sha, nil
}

// AddCommit links a commit as a human; see AddCommitAs.
func (s *Store) AddCommit(ctx context.Context, wsID, issueID, sha, message string, url *string) (models.IssueCommit, error) {
	return s.AddCommitAs(ctx, wsID, issueID, sha, message, url, "")
}

// AddCommitAs links a commit to an issue and records commit_linked on its
// timeline in the same transaction.
func (s *Store) AddCommitAs(ctx context.Context, wsID, issueID, sha, message string, url *string, actor string) (models.IssueCommit, error) {
	sha, err := NormalizeSHA(sha)
	if err != nil {
		return models.IssueCommit{}, err
	}
	url, err = normURL("url", url)
	if err != nil {
		return models.IssueCommit{}, err
	}
	// No explicit URL? Build one from the issue's default repo (Epic/Project).
	if url == nil {
		if built := commitURL(s.IssueRepo(ctx, wsID, issueID), sha); built != "" && models.ValidateHTTPURL(built) == nil {
			url = &built
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.IssueCommit{}, err
	}
	defer tx.Rollback(ctx)
	var c models.IssueCommit
	err = tx.QueryRow(ctx,
		`INSERT INTO issue_commits (issue_id, sha, message, url)
		 SELECT $1,$2,$3,$4 FROM issues WHERE id=$1 AND workspace_id=$5
		 ON CONFLICT (issue_id, sha) DO NOTHING
		 RETURNING id, issue_id, sha, message, url, created_at`,
		issueID, sha, message, url, wsID).Scan(&c.ID, &c.IssueID, &c.SHA, &c.Message, &c.URL, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the issue is not in this workspace, or the commit is already
		// linked: link_commit is idempotent, so hand back the existing row and
		// leave the timeline alone.
		err = tx.QueryRow(ctx, `
			SELECT ic.id, ic.issue_id, ic.sha, ic.message, ic.url, ic.created_at
			FROM issue_commits ic JOIN issues i ON i.id = ic.issue_id
			WHERE ic.issue_id=$1 AND ic.sha=$2 AND i.workspace_id=$3`, issueID, sha, wsID).
			Scan(&c.ID, &c.IssueID, &c.SHA, &c.Message, &c.URL, &c.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return c, ErrNotFound
		}
		return c, err
	}
	if err != nil {
		return c, err
	}
	is, err := s.getIssueTx(ctx, tx, wsID, issueID, false)
	if err != nil {
		return c, err
	}
	if err := insertActivity(ctx, tx, wsID, models.Activity{
		IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor,
		Kind: "commit_linked", Field: "commit", ToVal: sha, Detail: message,
	}); err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

func (s *Store) ListCommits(ctx context.Context, wsID, issueID string) ([]models.IssueCommit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ic.id, ic.issue_id, ic.sha, ic.message, ic.url, ic.created_at
		FROM issue_commits ic JOIN issues i ON i.id = ic.issue_id
		WHERE ic.issue_id=$1 AND i.workspace_id=$2
		ORDER BY ic.created_at DESC`, issueID, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.IssueCommit
	for rows.Next() {
		var c models.IssueCommit
		if err := rows.Scan(&c.ID, &c.IssueID, &c.SHA, &c.Message, &c.URL, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- done-when criteria ----

// criterionCols is the column list every criterion query selects, in the order
// scanCriterion expects.
const criterionCols = `c.id, c.issue_id, c.body, c.done, c.position, c.kind, c.check_spec, c.evidence_ref, c.created_at`

func scanCriterion(row pgx.Row) (models.Criterion, error) {
	var c models.Criterion
	// check_spec is nullable jsonb; scan through []byte so NULL lands as nil
	// rather than an empty, invalid json.RawMessage.
	var spec []byte
	err := row.Scan(&c.ID, &c.IssueID, &c.Body, &c.Done, &c.Position,
		&c.Kind, &spec, &c.EvidenceRef, &c.CreatedAt)
	if len(spec) > 0 {
		c.CheckSpec = json.RawMessage(spec)
	}
	return c, err
}

func (s *Store) ListCriteria(ctx context.Context, wsID, issueID string) ([]models.Criterion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+criterionCols+`
		FROM issue_criteria c JOIN issues i ON i.id = c.issue_id
		WHERE c.issue_id=$1 AND i.workspace_id=$2
		ORDER BY c.position, c.created_at`, issueID, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Criterion
	for rows.Next() {
		c, err := scanCriterion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddCriterion appends a criterion. kind must be one of the models.Criterion*
// constants; checkSpec is required for every kind except manual.
func (s *Store) AddCriterion(ctx context.Context, wsID, issueID, body, kind string, checkSpec json.RawMessage) (models.Criterion, error) {
	if kind == "" {
		kind = models.CriterionManual
	}
	var spec []byte
	if len(checkSpec) > 0 {
		spec = checkSpec
	}
	c, err := scanCriterion(s.pool.QueryRow(ctx,
		`INSERT INTO issue_criteria (issue_id, body, position, kind, check_spec)
		 SELECT $1, $2, coalesce((SELECT max(position)+1 FROM issue_criteria WHERE issue_id=$1), 0), $4, $5
		 FROM issues WHERE id=$1 AND workspace_id=$3
		 RETURNING `+strings.ReplaceAll(criterionCols, "c.", "")+``, issueID, body, wsID, kind, spec))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// UpdateCriterion patches a criterion as a human; see UpdateCriterionAs.
func (s *Store) UpdateCriterion(ctx context.Context, wsID, id string, body *string, done *bool, kind *string, checkSpec json.RawMessage, evidenceRef *string) (models.Criterion, error) {
	return s.UpdateCriterionAs(ctx, wsID, id, body, done, kind, checkSpec, evidenceRef, "")
}

// UpdateCriterionAs patches a criterion. Every pointer/slice argument is
// optional; nil leaves that column untouched. When done flips, a
// criterion_checked row (criterion text, done true/false) goes on the issue's
// timeline in the same transaction.
func (s *Store) UpdateCriterionAs(ctx context.Context, wsID, id string, body *string, done *bool, kind *string, checkSpec json.RawMessage, evidenceRef *string, actor string) (models.Criterion, error) {
	var spec []byte
	if len(checkSpec) > 0 {
		spec = checkSpec
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Criterion{}, err
	}
	defer tx.Rollback(ctx)
	var wasDone bool
	var issueID, issueKey, issueTitle string
	err = tx.QueryRow(ctx, `
		SELECT c.done, c.issue_id, i.key, i.title
		FROM issue_criteria c JOIN issues i ON i.id = c.issue_id
		WHERE c.id=$1 AND i.workspace_id=$2 FOR UPDATE OF c`, id, wsID).Scan(&wasDone, &issueID, &issueKey, &issueTitle)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Criterion{}, ErrNotFound
	}
	if err != nil {
		return models.Criterion{}, err
	}
	// Evidence belongs to the tick. Un-ticking a criterion clears it, because a
	// criterion that reads "not met" while still citing a previous run's evidence
	// is a record that lies.
	c, err := scanCriterion(tx.QueryRow(ctx, `
		UPDATE issue_criteria c SET body=coalesce($2,c.body), done=coalesce($3,c.done),
			kind=coalesce($5,c.kind), check_spec=coalesce($6,c.check_spec),
			evidence_ref = CASE
				WHEN $3 IS NOT NULL AND $3 = false THEN $7
				ELSE coalesce($7, c.evidence_ref)
			END
		FROM issues i
		WHERE c.id=$1 AND i.id = c.issue_id AND i.workspace_id=$4
		RETURNING `+criterionCols, id, body, done, wsID, kind, spec, evidenceRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if c.Done != wasDone {
		if err := insertActivity(ctx, tx, wsID, criterionRow(issueID, issueKey, issueTitle, actor, c.Body, c.Done)); err != nil {
			return c, err
		}
	}
	return c, tx.Commit(ctx)
}

// criterionRow is the timeline row for a criterion being ticked or unticked.
func criterionRow(issueID, key, title, actor, text string, done bool) models.Activity {
	return models.Activity{
		IssueID: &issueID, IssueKey: key, IssueTitle: title, Actor: actor,
		Kind: "criterion_checked", Field: "done", FromVal: text, ToVal: fmt.Sprint(done),
	}
}

// ValidateCriterionSpec enforces the same rule as the DB constraint: anything
// other than a manual criterion has to say how it gets verified.
func ValidateCriterionSpec(kind string, spec json.RawMessage) error {
	switch kind {
	case "", models.CriterionManual:
		return nil
	case models.CriterionDeterministic, models.CriterionPolicy, models.CriterionJudgment:
		if len(spec) == 0 {
			return fmt.Errorf("checkSpec required for kind %q", kind)
		}
		if !json.Valid(spec) {
			return fmt.Errorf("checkSpec is not valid JSON")
		}
		return nil
	default:
		return fmt.Errorf("unknown criterion kind %q", kind)
	}
}

// CriterionInput is one line of a checklist handed to ReplaceCriteria.
type CriterionInput struct {
	Body string
	Done bool
	Kind string // empty means manual
	// Check is the raw JSON verification spec, nil for manual criteria.
	Check json.RawMessage
}

// ReplaceCriteria makes an issue's checklist exactly items, in order. It runs in
// one transaction with the issue row locked, so a failure leaves the stored list
// untouched and a concurrent AddCriterion cannot race the positions. Existing
// rows are reconciled in place (slot i is updated, extra slots appended, the
// tail deleted), so ticking one item off re-sends the same list without churning
// ids or created_at. Each criterion whose done flag changed (a new item that
// arrives ticked counts) gets a criterion_checked row in the same transaction.
func (s *Store) ReplaceCriteria(ctx context.Context, wsID, issueID string, items []CriterionInput, actor string) ([]models.Criterion, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var issueKey, issueTitle string
	if err := tx.QueryRow(ctx,
		`SELECT key, title FROM issues WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, issueID, wsID).Scan(&issueKey, &issueTitle); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT `+criterionCols+` FROM issue_criteria c
		WHERE c.issue_id=$1 ORDER BY c.position, c.created_at`, issueID)
	if err != nil {
		return nil, err
	}
	var existing []models.Criterion
	for rows.Next() {
		c, err := scanCriterion(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		existing = append(existing, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.Criterion, 0, len(items))
	for i, it := range items {
		kind := it.Kind
		if kind == "" {
			kind = models.CriterionManual
		}
		var spec []byte
		if len(it.Check) > 0 {
			spec = it.Check
		}
		var c models.Criterion
		if i < len(existing) {
			// Un-ticking drops the evidence, as UpdateCriterion does.
			c, err = scanCriterion(tx.QueryRow(ctx, `
				UPDATE issue_criteria c SET body=$2, done=$3, position=$4, kind=$5, check_spec=$6,
					evidence_ref = CASE WHEN $3 THEN c.evidence_ref ELSE NULL END
				WHERE c.id=$1
				RETURNING `+criterionCols, existing[i].ID, it.Body, it.Done, i, kind, spec))
		} else {
			c, err = scanCriterion(tx.QueryRow(ctx, `
				INSERT INTO issue_criteria (issue_id, body, done, position, kind, check_spec)
				VALUES ($1,$2,$3,$4,$5,$6)
				RETURNING `+strings.ReplaceAll(criterionCols, "c.", ""), issueID, it.Body, it.Done, i, kind, spec))
		}
		if err != nil {
			return nil, err
		}
		wasDone := i < len(existing) && existing[i].Done
		if c.Done != wasDone {
			if err := insertActivity(ctx, tx, wsID, criterionRow(issueID, issueKey, issueTitle, actor, c.Body, c.Done)); err != nil {
				return nil, err
			}
		}
		out = append(out, c)
	}
	if len(existing) > len(items) {
		stale := make([]string, 0, len(existing)-len(items))
		for _, c := range existing[len(items):] {
			stale = append(stale, c.ID)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM issue_criteria WHERE id = ANY($1::uuid[])`, stale); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) DeleteCriterion(ctx context.Context, wsID, id string) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM issue_criteria c USING issues i
		WHERE c.id=$1 AND i.id = c.issue_id AND i.workspace_id=$2`, id, wsID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CommitOwner is the issue a commit belongs to, with the done-when list
// that commit was meant to satisfy.
//
// This is the reverse of AddCommit, and it exists so a code-intelligence
// tool can start from a commit SHA — the one identifier both systems
// already record — and recover the intent behind it. DoneWhen knows WHY code
// was written; a code-analysis tool knows WHAT it actually does. Matching the
// two is what turns "the ticket says it is done" into "the code shows it is
// done".
type CommitOwner struct {
	IssueID   string             `json:"issueId"`
	IssueKey  string             `json:"issueKey"`
	Title     string             `json:"title"`
	State     string             `json:"state"`
	SHA       string             `json:"sha"`
	Message   string             `json:"message"`
	URL       *string            `json:"url"`
	Criteria  []models.Criterion `json:"criteria"`
	CreatedAt time.Time          `json:"createdAt"`
}

// IssueByCommit returns the issue that recorded sha, with its acceptance
// criteria, searched across the given workspaces.
//
// sha must be 7 to 40 hex characters (any case). It matches stored shas that
// start with it, so a short sha (git rev-parse --short) finds the full one a
// ticket recorded. A stored short sha does not match a longer query. Returns
// ErrNotFound when no issue claims the commit — an ordinary outcome, since
// plenty of commits are not tracked.
func (s *Store) IssueByCommit(ctx context.Context, wsIDs []string, sha string) (CommitOwner, error) {
	var out CommitOwner
	sha, err := NormalizeSHA(sha)
	if err != nil {
		return out, err
	}
	var wsID string
	err = s.pool.QueryRow(ctx, `
		SELECT i.id, i.key, i.title, COALESCE(ws.name, ''), i.workspace_id,
		       ic.sha, ic.message, ic.url, ic.created_at
		FROM issue_commits ic
		JOIN issues i ON i.id = ic.issue_id
		LEFT JOIN workflow_states ws ON ws.id = i.state_id
		WHERE i.workspace_id = ANY($2)
		  AND left(ic.sha, length($1)) = $1
		ORDER BY ic.created_at DESC
		LIMIT 1
	`, sha, wsIDs).Scan(&out.IssueID, &out.IssueKey, &out.Title, &out.State, &wsID,
		&out.SHA, &out.Message, &out.URL, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	crit, err := s.ListCriteria(ctx, wsID, out.IssueID)
	if err != nil {
		return out, err
	}
	out.Criteria = crit
	return out, nil
}
