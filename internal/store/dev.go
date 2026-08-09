package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// ---- dev links (branch / PR) ----

// SetIssueDev sets the branch + PR that implemented an issue (nil clears).
func (s *Store) SetIssueDev(ctx context.Context, wsID, issueID string, branch, prURL *string) (models.Issue, error) {
	ct, err := s.pool.Exec(ctx,
		`UPDATE issues SET git_branch=$3, pr_url=$4, updated_at=now() WHERE id=$1 AND workspace_id=$2`,
		issueID, wsID, branch, prURL)
	if err != nil {
		return models.Issue{}, err
	}
	if ct.RowsAffected() == 0 {
		return models.Issue{}, ErrNotFound
	}
	return s.GetIssue(ctx, wsID, issueID)
}

// IssueRepo returns the default repo for an issue — its Epic's repo_url, else
// its Project's (initiative's) repo_url, else "".
func (s *Store) IssueRepo(ctx context.Context, wsID, issueID string) string {
	var repo *string
	err := s.pool.QueryRow(ctx, `
		SELECT coalesce(p.repo_url, i.repo_url)
		FROM issues iss
		LEFT JOIN projects p ON p.id = iss.project_id
		LEFT JOIN initiatives i ON i.id = p.initiative_id
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

func (s *Store) AddCommit(ctx context.Context, wsID, issueID, sha, message string, url *string) (models.IssueCommit, error) {
	// No explicit URL? Build one from the issue's default repo (Epic/Project).
	if url == nil || *url == "" {
		if built := commitURL(s.IssueRepo(ctx, wsID, issueID), sha); built != "" {
			url = &built
		}
	}
	var c models.IssueCommit
	err := s.pool.QueryRow(ctx,
		`INSERT INTO issue_commits (issue_id, sha, message, url)
		 SELECT $1,$2,$3,$4 FROM issues WHERE id=$1 AND workspace_id=$5
		 RETURNING id, issue_id, sha, message, url, created_at`,
		issueID, sha, message, url, wsID).Scan(&c.ID, &c.IssueID, &c.SHA, &c.Message, &c.URL, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
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

func scanCriterion(row pgx.Row) (models.Criterion, error) {
	var c models.Criterion
	err := row.Scan(&c.ID, &c.IssueID, &c.Body, &c.Done, &c.Position, &c.CreatedAt)
	return c, err
}

func (s *Store) ListCriteria(ctx context.Context, wsID, issueID string) ([]models.Criterion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.issue_id, c.body, c.done, c.position, c.created_at
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

func (s *Store) AddCriterion(ctx context.Context, wsID, issueID, body string) (models.Criterion, error) {
	c, err := scanCriterion(s.pool.QueryRow(ctx,
		`INSERT INTO issue_criteria (issue_id, body, position)
		 SELECT $1, $2, coalesce((SELECT max(position)+1 FROM issue_criteria WHERE issue_id=$1), 0)
		 FROM issues WHERE id=$1 AND workspace_id=$3
		 RETURNING id, issue_id, body, done, position, created_at`, issueID, body, wsID))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) UpdateCriterion(ctx context.Context, wsID, id string, body *string, done *bool) (models.Criterion, error) {
	c, err := scanCriterion(s.pool.QueryRow(ctx, `
		UPDATE issue_criteria c SET body=coalesce($2,c.body), done=coalesce($3,c.done)
		FROM issues i
		WHERE c.id=$1 AND i.id = c.issue_id AND i.workspace_id=$4
		RETURNING c.id, c.issue_id, c.body, c.done, c.position, c.created_at`, id, body, done, wsID))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
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
// already record — and recover the intent behind it. Raenil knows WHY code
// was written; a tool like Lumos knows WHAT it actually does. Matching the
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
// criteria, scoped to one workspace.
//
// Matches on prefix in both directions so a short SHA (git rev-parse
// --short, which is what most tools record) finds a full one and vice
// versa. Returns pgx.ErrNoRows when no issue claims the commit — an
// ordinary outcome, since plenty of commits are not tracked.
func (s *Store) IssueByCommit(ctx context.Context, wsID, sha string) (CommitOwner, error) {
	var out CommitOwner
	err := s.pool.QueryRow(ctx, `
		SELECT i.id, i.key, i.title, COALESCE(ws.name, ''),
		       ic.sha, ic.message, ic.url, ic.created_at
		FROM issue_commits ic
		JOIN issues i ON i.id = ic.issue_id
		LEFT JOIN workflow_states ws ON ws.id = i.state_id
		WHERE i.workspace_id = $2
		  AND (ic.sha = $1 OR ic.sha LIKE $1 || '%' OR $1 LIKE ic.sha || '%')
		ORDER BY length(ic.sha) DESC, ic.created_at DESC
		LIMIT 1
	`, sha, wsID).Scan(&out.IssueID, &out.IssueKey, &out.Title, &out.State,
		&out.SHA, &out.Message, &out.URL, &out.CreatedAt)
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
