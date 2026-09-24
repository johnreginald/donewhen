package mcp

import (
	"fmt"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"

	"raenil/internal/models"
)

// A list answers "which one?", not "what does it say?". Its rows carry enough
// to choose one and name it in the next call; the markdown bodies, uuids and
// colours that made a single list_issues call cost megabytes stay behind the
// get_* tools, or behind verbose:true for the rare caller that wants them.

// defaultListLimit caps a list that was not given one. An uncapped list over
// every workspace is never what a model meant to ask for.
const defaultListLimit = 50

// dateOnly is the precision a list needs to tell recent from stale.
const dateOnly = "2006-01-02"

func verboseArg() mcp.ToolOption {
	return mcp.WithBoolean("verbose",
		mcp.Description("Return full objects, bodies included. Costly — prefer the get_* tool for the one you need."))
}

func limitArg() mcp.ToolOption {
	return mcp.WithNumber("limit",
		mcp.Description(fmt.Sprintf("Max rows (default %d)", defaultListLimit)))
}

// listLimit reads the caller's limit, falling back to the default.
func listLimit(req mcp.CallToolRequest) int {
	if n := req.GetInt("limit", 0); n > 0 {
		return n
	}
	return defaultListLimit
}

// page trims rows to limit and wraps them under name, adding a note when rows
// were left out so the model knows to narrow instead of assuming it saw all.
// Callers fetch limit+1 so that note is exact.
func page[T any](name string, rows []T, limit int) map[string]any {
	out := map[string]any{}
	if len(rows) > limit {
		rows = rows[:limit]
		out["more"] = fmt.Sprintf("showing %d; narrow the filters or raise limit to see the rest", limit)
	}
	if rows == nil {
		rows = []T{}
	}
	out[name] = rows
	return out
}

type issueRow struct {
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	State      string   `json:"state"`
	Priority   int      `json:"priority,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	ParentKey  *string  `json:"parentKey,omitempty"`
	ProjectID  *string  `json:"projectId,omitempty"`
	ChildCount int      `json:"childCount,omitempty"`
	DocCount   int      `json:"docCount,omitempty"`
	Updated    string   `json:"updated"`
}

// issueRows projects issues to list rows. states maps state id to name; the
// name is what every other tool takes, the id is what the row used to carry.
func issueRows(issues []models.Issue, states map[string]string) []issueRow {
	out := make([]issueRow, len(issues))
	for i, is := range issues {
		out[i] = issueRow{
			Key:        is.Key,
			Title:      is.Title,
			State:      states[is.StateID],
			Priority:   is.Priority,
			Labels:     labelNames(is.Labels),
			ParentKey:  is.ParentKey,
			ProjectID:  is.ProjectID,
			ChildCount: is.ChildCount,
			DocCount:   is.DocCount,
			Updated:    is.UpdatedAt.Format(dateOnly),
		}
	}
	return out
}

type documentRow struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	Author       string   `json:"author"`
	IssueID      *string  `json:"issueId,omitempty"`
	ProjectID    *string  `json:"projectId,omitempty"`
	InitiativeID *string  `json:"initiativeId,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Updated      string   `json:"updated"`
}

func documentRows(docs []models.Document) []documentRow {
	out := make([]documentRow, len(docs))
	for i, d := range docs {
		out[i] = documentRow{
			ID:           d.ID,
			Title:        d.Title,
			Type:         d.Type,
			Author:       d.Author,
			IssueID:      d.IssueID,
			ProjectID:    d.ProjectID,
			InitiativeID: d.InitiativeID,
			Labels:       labelNames(d.Labels),
			Updated:      d.UpdatedAt.Format(dateOnly),
		}
	}
	return out
}

type projectRow struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	InitiativeID *string `json:"initiativeId,omitempty"`
	RepoURL      *string `json:"repoUrl,omitempty"`
}

func projectRows(ps []models.Project) []projectRow {
	out := make([]projectRow, len(ps))
	for i, p := range ps {
		out[i] = projectRow{ID: p.ID, Name: p.Name, Status: p.Status, InitiativeID: p.InitiativeID, RepoURL: p.RepoURL}
	}
	return out
}

// labelGroupRow is one group's labels, merged across workspaces by name:
// save_issue and list_issues take label names, so the per-workspace ids and
// colours are noise to a model.
type labelGroupRow struct {
	Exclusive bool     `json:"exclusive,omitempty"`
	Labels    []string `json:"labels"`
}

// labelCatalog groups label names under their group's name. Labels with no
// group sit under "".
func labelCatalog(labels []models.Label, groups []models.LabelGroup) map[string]*labelGroupRow {
	type g struct {
		name      string
		exclusive bool
	}
	byID := make(map[string]g, len(groups))
	for _, gr := range groups {
		byID[gr.ID] = g{gr.Name, gr.Exclusive}
	}
	out := map[string]*labelGroupRow{}
	seen := map[[2]string]bool{}
	for _, l := range labels {
		var grp g
		if l.GroupID != nil {
			grp = byID[*l.GroupID]
		}
		if seen[[2]string{grp.name, l.Name}] {
			continue
		}
		seen[[2]string{grp.name, l.Name}] = true
		row, ok := out[grp.name]
		if !ok {
			row = &labelGroupRow{}
			out[grp.name] = row
		}
		row.Exclusive = row.Exclusive || grp.exclusive
		row.Labels = append(row.Labels, l.Name)
	}
	for _, row := range out {
		sort.Strings(row.Labels)
	}
	return out
}

func labelNames(ls []models.Label) []string {
	if len(ls) == 0 {
		return nil
	}
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Name
	}
	return out
}
