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
		mcp.WithDescription("Link a commit to an issue — the record of HOW it was done. "+
			"Call this when you finish work so the issue points at the actual code."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key (e.g. R-8)")),
		mcp.WithString("sha", mcp.Required(), mcp.Description("Commit SHA")),
		mcp.WithString("message", mcp.Description("Commit subject line")),
		mcp.WithString("url", mcp.Description("Link to the commit (optional)")),
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

	// ---- set_criteria ----
	s.AddTool(mcp.NewTool("set_criteria",
		mcp.WithDescription("Set an issue's done-when acceptance checklist (replaces the list). "+
			"Use during Aligning to lock what 'done' means."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
		mcp.WithArray("items", mcp.Required(), mcp.Description("Acceptance criteria, one per item"),
			mcp.Items(map[string]any{"type": "string"})),
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
		for _, body := range stringSlice(req, "items") {
			c, err := d.store.AddCriterion(ctx, is.ID, body)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
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
