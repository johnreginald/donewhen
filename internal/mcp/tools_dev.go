package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"raenil/internal/models"
)

// registerDev exposes the "how it was done" + "done-when" surface: link commits,
// set the branch/PR, and manage acceptance criteria. This is how Claude keeps
// the record as it works.
func (d *deps) registerDev(s *server.MCPServer) {
	// ---- link_commit ----
	s.AddTool(mcp.NewTool("link_commit",
		mcp.WithDescription("Link a commit to an issue — the record of HOW it was done. Call this "+
			"when you finish work, reusing the repo you committed in: get sha from `git rev-parse "+
			"HEAD` and the commit URL from `git remote get-url origin` (→ <repo>/commit/<sha>). "+
			"If url is omitted, it's auto-built from the issue's Epic/Project default repo. Pass the "+
			"full url for cross-repo work."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key (e.g. R-8)")),
		mcp.WithString("sha", mcp.Required(), mcp.Description("Commit SHA (git rev-parse HEAD)")),
		mcp.WithString("message", mcp.Description("Commit subject line")),
		mcp.WithString("url", mcp.Description("Full commit URL; omit to auto-build from the project repo")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		is, err := d.resolveIssueRef(ctx, req.GetString("issue", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		c, err := d.store.AddCommit(ctx, is.ID, req.GetString("sha", ""), req.GetString("message", ""), strp(req.GetString("url", "")))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		_ = d.store.RecordActivity(ctx, models.Activity{
			IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: "ai",
			Kind: "committed", Detail: shortSHA(c.SHA) + " " + c.Message,
		})
		return jsonResult(c)
	})

	// ---- set_issue_dev ----
	s.AddTool(mcp.NewTool("set_issue_dev",
		mcp.WithDescription("Set the branch and/or pull-request URL that implemented an issue."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
		mcp.WithString("gitBranch", mcp.Description("Branch name")),
		mcp.WithString("prUrl", mcp.Description("Pull request URL")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		is, err := d.resolveIssueRef(ctx, req.GetString("issue", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		upd, err := d.store.SetIssueDev(ctx, is.ID, strp(req.GetString("gitBranch", "")), strp(req.GetString("prUrl", "")))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(upd)
	})

	// ---- get_criteria ----
	s.AddTool(mcp.NewTool("get_criteria",
		mcp.WithDescription("Get an issue's done-when acceptance checklist with each item's done state. "+
			"Read this before moving an issue toward Done — every criterion must be done:true."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		is, err := d.resolveIssueRef(ctx, req.GetString("issue", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		items, err := d.store.ListCriteria(ctx, is.ID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if items == nil {
			items = []models.Criterion{}
		}
		return jsonResult(items)
	})

	// ---- set_criteria ----
	s.AddTool(mcp.NewTool("set_criteria",
		mcp.WithDescription("Set an issue's done-when acceptance checklist. Declarative: re-send the FULL "+
			"list every call (it replaces what's stored). Define the criteria during Aligning; then as each "+
			"one is met, re-send the same list with that item's done:true. Don't move an issue to Done until "+
			"every item is done:true."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
		mcp.WithArray("items", mcp.Required(),
			mcp.Description("Ordered checklist. Each item is {text, done}; done defaults false. "+
				"Plain strings also accepted (treated as not-done)."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "Criterion text"},
					"done": map[string]any{"type": "boolean", "description": "Met yet? default false"},
				},
				"required": []any{"text"},
			})),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		is, err := d.resolveIssueRef(ctx, req.GetString("issue", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		existing, _ := d.store.ListCriteria(ctx, is.ID)
		for _, c := range existing {
			_ = d.store.DeleteCriterion(ctx, c.ID)
		}
		out := []models.Criterion{}
		for _, it := range criteriaItems(req) {
			c, err := d.store.AddCriterion(ctx, is.ID, it.text)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if it.done {
				done := true
				if c, err = d.store.UpdateCriterion(ctx, c.ID, nil, &done); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
			}
			out = append(out, c)
		}
		return jsonResult(out)
	})

	// ---- get_activity ----
	s.AddTool(mcp.NewTool("get_activity",
		mcp.WithDescription("Get an issue's activity timeline (who did what, when) — the record."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		is, err := d.resolveIssueRef(ctx, req.GetString("issue", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		acts, err := d.store.ListActivity(ctx, is.ID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(acts)
	})
}

// shortSHA duplicated small helper (mcp package has no access to api's).
func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
