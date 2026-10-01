package store

import (
	"context"
	"os"
	"testing"

	"raenil/internal/models"
)

func TestLinkURLsAreHTTPOnly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "x", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	str := func(v string) *string { return &v }

	for _, bad := range []string{"javascript:alert(1)", "data:text/html,x", "vbscript:x", "file:///etc/passwd", "github.com/x/y"} {
		_, err := s.SetIssueDev(ctx, ws, is.ID, nil, str(bad))
		wantInvalid(t, err, "invalid_url")
		_, err = s.AddCommit(ctx, ws, is.ID, "abc1234", "m", str(bad))
		wantInvalid(t, err, "invalid_url")
		_, err = s.SaveProject(ctx, ws, models.Project{Name: "p", RepoURL: str(bad)})
		wantInvalid(t, err, "invalid_url")
		_, err = s.SaveInitiative(ctx, ws, models.Initiative{Name: "i", RepoURL: str(bad)})
		wantInvalid(t, err, "invalid_url")
	}
	if cs, _ := s.ListCommits(ctx, ws, is.ID); len(cs) != 0 {
		t.Fatalf("rejected commit was stored: %v", cs)
	}

	pr := "https://github.com/x/y/pull/1"
	got, err := s.SetIssueDev(ctx, ws, is.ID, nil, &pr)
	if err != nil || got.PRURL == nil || *got.PRURL != pr {
		t.Fatalf("valid PR url: %v %v", got.PRURL, err)
	}
	// Empty clears.
	got, err = s.SetIssueDev(ctx, ws, is.ID, nil, str(""))
	if err != nil || got.PRURL != nil {
		t.Fatalf("empty should clear: %v %v", got.PRURL, err)
	}
	c, err := s.AddCommit(ctx, ws, is.ID, "abc1234", "m", str("https://github.com/x/y/commit/abc1234"))
	if err != nil || c.URL == nil {
		t.Fatalf("valid commit url: %v", err)
	}
	// Repo URL feeds the auto-built commit link.
	p, err := s.SaveProject(ctx, ws, models.Project{Name: "p", RepoURL: str("https://github.com/x/y")})
	if err != nil {
		t.Fatal(err)
	}
	is2, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "y", StateName: "Backlog", ProjectID: &p.ID})
	c, err = s.AddCommit(ctx, ws, is2.ID, "def5678", "m", nil)
	if err != nil || c.URL == nil || *c.URL != "https://github.com/x/y/commit/def5678" {
		t.Fatalf("auto-built url: %v %v", c.URL, err)
	}
}

func TestMigrationNullsNonHTTPURLs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "x", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.SaveProject(ctx, ws, models.Project{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	in, err := s.SaveInitiative(ctx, ws, models.Initiative{Name: "i"})
	if err != nil {
		t.Fatal(err)
	}
	// Legacy rows written before validation existed.
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE issues SET pr_url='javascript:alert(1)' WHERE id=$1`, []any{is.ID}},
		{`UPDATE projects SET repo_url='data:text/html,x' WHERE id=$1`, []any{p.ID}},
		{`UPDATE initiatives SET repo_url='github.com/x/y' WHERE id=$1`, []any{in.ID}},
		{`INSERT INTO issue_commits (issue_id, sha, message, url) VALUES ($1,'bad0001','m','JavaScript:alert(1)')`, []any{is.ID}},
		{`INSERT INTO issue_commits (issue_id, sha, message, url) VALUES ($1,'good001','m','https://github.com/x/y/commit/good001')`, []any{is.ID}},
	} {
		if _, err := s.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	sqlBytes, err := os.ReadFile("../db/migrations/0038_http_urls_only.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatal(err)
	}
	var pr, repo, irepo *string
	_ = s.pool.QueryRow(ctx, `SELECT pr_url FROM issues WHERE id=$1`, is.ID).Scan(&pr)
	_ = s.pool.QueryRow(ctx, `SELECT repo_url FROM projects WHERE id=$1`, p.ID).Scan(&repo)
	_ = s.pool.QueryRow(ctx, `SELECT repo_url FROM initiatives WHERE id=$1`, in.ID).Scan(&irepo)
	if pr != nil || repo != nil || irepo != nil {
		t.Fatalf("bad urls survived: %v %v %v", pr, repo, irepo)
	}
	cs, err := s.ListCommits(ctx, ws, is.ID)
	if err != nil || len(cs) != 2 {
		t.Fatalf("commits = %v, %v (rows must be kept)", cs, err)
	}
	for _, c := range cs {
		switch c.SHA {
		case "bad0001":
			if c.URL != nil {
				t.Errorf("bad commit url kept: %v", *c.URL)
			}
		case "good001":
			if c.URL == nil {
				t.Error("good commit url was nulled")
			}
		}
	}
}
