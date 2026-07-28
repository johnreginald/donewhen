package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// ---- dev links (branch / PR) ----

// SetIssueDev sets the branch + PR that implemented an issue (nil clears).
func (s *Store) SetIssueDev(ctx context.Context, issueID string, branch, prURL *string) (models.Issue, error) {
	ct, err := s.pool.Exec(ctx,
		`UPDATE issues SET git_branch=$2, pr_url=$3, updated_at=now() WHERE id=$1`, issueID, branch, prURL)
	if err != nil {
		return models.Issue{}, err
	}
	if ct.RowsAffected() == 0 {
		return models.Issue{}, ErrNotFound
	}
	return s.GetIssue(ctx, issueID)
}

// IssueRepo returns the default repo for an issue — its Epic's repo_url, else
// its Project's (initiative's) repo_url, else "".
func (s *Store) IssueRepo(ctx context.Context, issueID string) string {
	var repo *string
	err := s.pool.QueryRow(ctx, `
		SELECT coalesce(p.repo_url, i.repo_url)
		FROM issues iss
		LEFT JOIN projects p ON p.id = iss.project_id
		LEFT JOIN initiatives i ON i.id = p.initiative_id
		WHERE iss.id = $1`, issueID).Scan(&repo)
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

func (s *Store) AddCommit(ctx context.Context, issueID, sha, message string, url *string) (models.IssueCommit, error) {
	// No explicit URL? Build one from the issue's default repo (Epic/Project).
	if url == nil || *url == "" {
		if built := commitURL(s.IssueRepo(ctx, issueID), sha); built != "" {
			url = &built
		}
	}
	var c models.IssueCommit
	err := s.pool.QueryRow(ctx,
		`INSERT INTO issue_commits (issue_id, sha, message, url) VALUES ($1,$2,$3,$4)
		 RETURNING id, issue_id, sha, message, url, created_at`,
		issueID, sha, message, url).Scan(&c.ID, &c.IssueID, &c.SHA, &c.Message, &c.URL, &c.CreatedAt)
	return c, err
}

func (s *Store) ListCommits(ctx context.Context, issueID string) ([]models.IssueCommit, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, issue_id, sha, message, url, created_at FROM issue_commits
		 WHERE issue_id=$1 ORDER BY created_at DESC`, issueID)
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

func (s *Store) ListCriteria(ctx context.Context, issueID string) ([]models.Criterion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, issue_id, body, done, position, created_at FROM issue_criteria
		 WHERE issue_id=$1 ORDER BY position, created_at`, issueID)
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

func (s *Store) AddCriterion(ctx context.Context, issueID, body string) (models.Criterion, error) {
	return scanCriterion(s.pool.QueryRow(ctx,
		`INSERT INTO issue_criteria (issue_id, body, position)
		 VALUES ($1, $2, coalesce((SELECT max(position)+1 FROM issue_criteria WHERE issue_id=$1), 0))
		 RETURNING id, issue_id, body, done, position, created_at`, issueID, body))
}

func (s *Store) UpdateCriterion(ctx context.Context, id string, body *string, done *bool) (models.Criterion, error) {
	c, err := scanCriterion(s.pool.QueryRow(ctx,
		`UPDATE issue_criteria SET body=coalesce($2,body), done=coalesce($3,done) WHERE id=$1
		 RETURNING id, issue_id, body, done, position, created_at`, id, body, done))
	if err == pgx.ErrNoRows {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) DeleteCriterion(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM issue_criteria WHERE id=$1`, id)
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
// was written; a tool like Globex knows WHAT it actually does. Matching the
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
// criteria.
//
// Matches on prefix in both directions so a short SHA (git rev-parse
// --short, which is what most tools record) finds a full one and vice
// versa. Returns pgx.ErrNoRows when no issue claims the commit — an
// ordinary outcome, since plenty of commits are not tracked.
func (s *Store) IssueByCommit(ctx context.Context, sha string) (CommitOwner, error) {
	var out CommitOwner
	err := s.pool.QueryRow(ctx, `
		SELECT i.id, i.key, i.title, COALESCE(ws.name, ''),
		       ic.sha, ic.message, ic.url, ic.created_at
		FROM issue_commits ic
		JOIN issues i ON i.id = ic.issue_id
		LEFT JOIN workflow_states ws ON ws.id = i.state_id
		WHERE ic.sha = $1 OR ic.sha LIKE $1 || '%' OR $1 LIKE ic.sha || '%'
		ORDER BY length(ic.sha) DESC, ic.created_at DESC
		LIMIT 1
	`, sha).Scan(&out.IssueID, &out.IssueKey, &out.Title, &out.State,
		&out.SHA, &out.Message, &out.URL, &out.CreatedAt)
	if err != nil {
		return out, err
	}
	crit, err := s.ListCriteria(ctx, out.IssueID)
	if err != nil {
		return out, err
	}
	out.Criteria = crit
	return out, nil
}
