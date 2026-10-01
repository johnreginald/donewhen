package mcp

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/johnreginald/donewhen/internal/models"
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
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, issueRef(req.GetString("issue", "")))
		if err != nil {
			return toolErr(err), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, req.GetString("issue", ""))
		if err != nil {
			return toolErr(err), nil
		}
		c, err := d.store.AddCommit(ctx, wsID, is.ID, req.GetString("sha", ""), req.GetString("message", ""), strp(req.GetString("url", "")))
		if err != nil {
			return toolErr(err), nil
		}
		_ = d.store.RecordActivity(ctx, wsID, models.Activity{
			IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: "ai",
			Kind: "committed", Detail: shortSHA(c.SHA) + " " + c.Message,
		})
		return jsonResult(c)
	})

	// ---- issue_by_commit ----
	s.AddTool(mcp.NewTool("issue_by_commit",
		mcp.WithDescription("Find the issue that recorded a commit SHA, with its done-when criteria. "+
			"The reverse of link_commit. Use it when you have a commit and need the intent behind "+
			"it — e.g. a code-intelligence tool reports what a commit actually changed, and you want "+
			"to check that against what the ticket said it should do. Matches short and full SHAs "+
			"in either direction. Returns not-found when no issue claims the commit, which is "+
			"ordinary: plenty of commits are untracked."),
		mcp.WithString("sha", mcp.Required(), mcp.Description("Commit SHA, short or full")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsIDs, err := d.scopeAll(ctx, req)
		if err != nil {
			return toolErr(err), nil
		}
		owner, err := d.store.IssueByCommit(ctx, wsIDs, req.GetString("sha", ""))
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(owner)
	})

	// ---- set_issue_dev ----
	s.AddTool(mcp.NewTool("set_issue_dev",
		mcp.WithDescription("Set the branch and/or pull-request URL that implemented an issue."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
		mcp.WithString("gitBranch", mcp.Description("Branch name")),
		mcp.WithString("prUrl", mcp.Description("Pull request URL")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, issueRef(req.GetString("issue", "")))
		if err != nil {
			return toolErr(err), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, req.GetString("issue", ""))
		if err != nil {
			return toolErr(err), nil
		}
		upd, err := d.store.SetIssueDev(ctx, wsID, is.ID, strp(req.GetString("gitBranch", "")), strp(req.GetString("prUrl", "")))
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(upd)
	})

	// ---- get_criteria ----
	s.AddTool(mcp.NewTool("get_criteria",
		mcp.WithDescription("Get an issue's done-when acceptance checklist with each item's done state. "+
			"Read this before moving an issue toward Done — every criterion must be done:true."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, issueRef(req.GetString("issue", "")))
		if err != nil {
			return toolErr(err), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, req.GetString("issue", ""))
		if err != nil {
			return toolErr(err), nil
		}
		items, err := d.store.ListCriteria(ctx, wsID, is.ID)
		if err != nil {
			return toolErr(err), nil
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
			mcp.Description("Ordered checklist. Each item is {text, done, kind, check}; done defaults false "+
				"and kind defaults \"manual\". Plain strings also accepted (treated as a not-done manual item)."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "Criterion text"},
					"done": map[string]any{"type": "boolean", "description": "Met yet? default false"},
					"kind": map[string]any{
						"type": "string",
						"enum": []any{"manual", "deterministic", "policy", "judgment"},
						"description": "How this criterion is verified. manual = a human ticks it. " +
							"deterministic = a command decides. policy = a rule over the diff decides. " +
							"judgment = a model opines (advisory only, never a sole gate on Done).",
					},
					"check": map[string]any{
						"type": "object",
						"description": "How to verify, required unless kind is manual. " +
							"deterministic: {\"cmd\":\"go test ./...\",\"expect_exit\":0}. " +
							"policy: {\"policy\":\"paths_within\",\"args\":[\"src/**\"]}. " +
							"judgment: {\"prompt\":\"...\",\"model\":\"...\"}.",
					},
				},
				"required": []any{"text"},
			})),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, issueRef(req.GetString("issue", "")))
		if err != nil {
			return toolErr(err), nil
		}
		// Validate the whole list before any write: a malformed items arg must
		// never wipe or half-write the checklist.
		items, err := criteriaItems(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, req.GetString("issue", ""))
		if err != nil {
			return toolErr(err), nil
		}
		out, err := d.store.ReplaceCriteria(ctx, wsID, is.ID, items)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(out)
	})

	// ---- check_criterion ----
	s.AddTool(mcp.NewTool("check_criterion",
		mcp.WithDescription("Tick (or untick) ONE done-when item in place — the light path for incremental "+
			"progress, no need to re-send the whole list. Identify the item by 1-based 'index' (its order in "+
			"get_criteria) or by exact case-insensitive 'text'. 'done' defaults true. Every other item and all "+
			"timestamps stay untouched."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
		mcp.WithNumber("index", mcp.Description("1-based position of the item in the checklist")),
		mcp.WithString("text", mcp.Description("Exact criterion text (case-insensitive) — alternative to index")),
		mcp.WithBoolean("done", mcp.Description("Met? default true")),
		mcp.WithString("evidence", mcp.Description("Reference to what verified this — e.g. an evidence id "+
			"or artifact path. Recorded alongside the tick so the record shows why it passed, not just that it did.")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, issueRef(req.GetString("issue", "")))
		if err != nil {
			return toolErr(err), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, req.GetString("issue", ""))
		if err != nil {
			return toolErr(err), nil
		}
		items, err := d.store.ListCriteria(ctx, wsID, is.ID)
		if err != nil {
			return toolErr(err), nil
		}
		var target *models.Criterion
		if idx := req.GetInt("index", 0); idx >= 1 && idx <= len(items) {
			target = &items[idx-1]
		} else if txt := strings.TrimSpace(req.GetString("text", "")); txt != "" {
			for i := range items {
				if strings.EqualFold(strings.TrimSpace(items[i].Body), txt) {
					target = &items[i]
					break
				}
			}
		}
		if target == nil {
			return mcp.NewToolResultError("no matching criterion — check index/text against get_criteria"), nil
		}
		done := req.GetBool("done", true)
		var evidence *string
		if e := strings.TrimSpace(req.GetString("evidence", "")); e != "" {
			evidence = &e
		}
		c, err := d.store.UpdateCriterion(ctx, wsID, target.ID, nil, &done, nil, nil, evidence)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(c)
	})

	// ---- get_activity ----
	s.AddTool(mcp.NewTool("get_activity",
		mcp.WithDescription("Get an issue's activity timeline (who did what, when) — the record."),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id or key")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, issueRef(req.GetString("issue", "")))
		if err != nil {
			return toolErr(err), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, req.GetString("issue", ""))
		if err != nil {
			return toolErr(err), nil
		}
		acts, err := d.store.ListActivity(ctx, wsID, is.ID)
		if err != nil {
			return toolErr(err), nil
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
