package mcp

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"raenil/internal/models"
)

// realisticIssue is shaped like a prod row: a few KB of markdown and the four
// or five labels a ticket carries under the exclusive-group taxonomy.
func realisticIssue(n int) models.Issue {
	project := "3f0c2a4e-8d6b-4c1a-9e2f-5b7d8a9c0e1f"
	parent := "ACM-100"
	grp := "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	labels := make([]models.Label, 0, 5)
	for _, name := range []string{"acme-server", "feature", "api", "ready-for-agent", "opencode"} {
		labels = append(labels, models.Label{ID: "c28a2dc0-4d73-4226-97c1-785f6c0c9b24", GroupID: &grp, Name: name, Color: "#94a3b8"})
	}
	return models.Issue{
		ID:            "9d1e2f3a-4b5c-4d6e-8f7a-1b2c3d4e5f6a",
		WorkspaceID:   "7a8b9c0d-1e2f-4a3b-9c4d-5e6f7a8b9c0d",
		Number:        n,
		Key:           fmt.Sprintf("ACM-%d", n),
		Title:         "Let a check see its own repository's test database",
		DescriptionMD: strings.Repeat("A paragraph of spec text with `code` and a ```mermaid block. ", 80),
		StateID:       "s-ready",
		ProjectID:     &project,
		Priority:      2,
		Labels:        labels,
		ParentKey:     &parent,
		CreatedAt:     time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 9, 24, 12, 34, 56, 789, time.UTC),
	}
}

func resultText(t *testing.T, v any) string {
	t.Helper()
	res, err := jsonResult(v)
	if err != nil {
		t.Fatal(err)
	}
	return res.Content[0].(mcp.TextContent).Text
}

func TestIssueListIsSmall(t *testing.T) {
	issues := make([]models.Issue, 20)
	for i := range issues {
		issues[i] = realisticIssue(100 + i)
	}
	text := resultText(t, page("issues", issueRows(issues, map[string]string{"s-ready": "Ready"}), 20))

	if len(text) >= 8*1024 {
		t.Errorf("20 issue rows are %d bytes, want < 8 KB", len(text))
	}
	for _, leaked := range []string{"descriptionMd", "workspaceId", "stateId", "color", "createdAt", "\n  "} {
		if strings.Contains(text, leaked) {
			t.Errorf("slim rows contain %q", leaked)
		}
	}
	for _, want := range []string{`"key":"ACM-100"`, `"state":"Ready"`, `"labels":["acme-server","feature"`, `"updated":"2026-09-24"`} {
		if !strings.Contains(text, want) {
			t.Errorf("slim rows lack %s", want)
		}
	}
}

func TestDocumentRowsCarryNoBody(t *testing.T) {
	docs := []models.Document{{ID: "d1", Title: "How it works", BodyMD: strings.Repeat("body ", 1000), Type: "change", Author: "ai"}}
	text := resultText(t, page("documents", documentRows(docs), defaultListLimit))
	if strings.Contains(text, "bodyMd") || strings.Contains(text, "body body") {
		t.Errorf("document rows carry the body: %s", text)
	}
}

func TestProjectRowsCarryNoDescription(t *testing.T) {
	text := resultText(t, projectRows([]models.Project{{ID: "p1", Name: "Epic", DescriptionMD: "long text", Status: "active"}}))
	if strings.Contains(text, "long text") {
		t.Errorf("project rows carry the description: %s", text)
	}
}

func TestPageNotesWhatItLeftOut(t *testing.T) {
	rows := []int{1, 2, 3}

	full := page("issues", rows, 3)
	if _, ok := full["more"]; ok {
		t.Errorf("a list that fits says there is more: %v", full)
	}

	cut := page("issues", rows, 2)
	if got := cut["issues"].([]int); len(got) != 2 {
		t.Errorf("kept %d rows, want 2", len(got))
	}
	if _, ok := cut["more"]; !ok {
		t.Errorf("a trimmed list does not say so: %v", cut)
	}

	if text := resultText(t, page[int]("issues", nil, 5)); text != `{"issues":[]}` {
		t.Errorf("empty list renders %s, want an empty array", text)
	}
}

func TestLabelCatalogMergesWorkspaces(t *testing.T) {
	repoA, repoB := "g-a", "g-b"
	groups := []models.LabelGroup{{ID: repoA, Name: "repo", Exclusive: true}, {ID: repoB, Name: "repo", Exclusive: true}}
	labels := []models.Label{
		{ID: "1", GroupID: &repoA, Name: "raenil"},
		{ID: "2", GroupID: &repoB, Name: "raenil"},
		{ID: "3", GroupID: &repoB, Name: "acme-server"},
		{ID: "4", Name: "api"},
	}
	text := resultText(t, labelCatalog(labels, groups))
	want := `{"":{"labels":["api"]},"repo":{"exclusive":true,"labels":["acme-server","raenil"]}}`
	if text != want {
		t.Errorf("catalog = %s\nwant      %s", text, want)
	}
}
