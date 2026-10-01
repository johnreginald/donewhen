package mcp

import (
	"context"
	"slices"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

func (d *deps) registerContent(s *server.MCPServer) {
	// ---- comments ----
	s.AddTool(mcp.NewTool("list_comments",
		mcp.WithDescription("List comments on an issue."),
		mcp.WithString("issueId", mcp.Required(), mcp.Description("Issue id or key")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref, err := req.RequireString("issueId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, issueRef(ref))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		comments, err := d.store.ListComments(ctx, wsID, is.ID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(comments)
	})

	s.AddTool(mcp.NewTool("save_comment",
		mcp.WithDescription("Add a comment to an issue."),
		mcp.WithString("issueId", mcp.Required(), mcp.Description("Issue id or key")),
		mcp.WithString("body", mcp.Required(), mcp.Description("Markdown comment body")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref, err := req.RequireString("issueId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		body, err := req.RequireString("body")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, issueRef(ref))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		c, err := d.svc.AddComment(ctx, wsID, is.ID, body, auth.ActorAI)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(c)
	})

	// ---- documents ----
	s.AddTool(mcp.NewTool("list_documents",
		mcp.WithDescription("List markdown documents, newest-updated first, optionally attached to a project, issue, "+
			"or initiative. Rows carry no body; use get_document to read one."),
		mcp.WithString("project", mcp.Description("Project id filter")),
		mcp.WithString("issue", mcp.Description("Issue id or key filter")),
		mcp.WithString("initiative", mcp.Description("Initiative id filter")),
		limitArg(),
		verboseArg(),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsIDs, err := d.scopeAll(ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		issueID := req.GetString("issue", "")
		if issueID != "" && !isUUID(issueID) {
			wsID, err := d.scopeOne(ctx, req, issueRef(issueID))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			is, err := d.resolveIssueRef(ctx, wsID, issueID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			issueID = is.ID
		}
		docs, err := d.store.ListDocuments(ctx, "", store.DocFilter{
			WorkspaceIDs: wsIDs,
			ProjectID:    req.GetString("project", ""),
			IssueID:      issueID,
			InitiativeID: req.GetString("initiative", ""),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := listLimit(req)
		if req.GetBool("verbose", false) {
			return jsonResult(page("documents", docs, limit))
		}
		return jsonResult(page("documents", documentRows(docs), limit))
	})

	s.AddTool(mcp.NewTool("get_document",
		mcp.WithDescription("Get a document by id."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Document id")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, documentRef(id))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		doc, err := d.store.GetDocument(ctx, wsID, id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(doc)
	})

	s.AddTool(mcp.NewTool("save_document",
		mcp.WithDescription("Write an engineering document to the knowledge base (create: omit id; update: pass id). "+
			"Documents are AI-authored: after implementing or changing something, record it here and attach it to the "+
			"issue you worked on (and/or its project).\n\n"+
			"Recommended structure for an implementation doc (type 'change'):\n"+
			"# <Feature / change title>\n"+
			"## Summary — what changed and why, in 2-3 sentences.\n"+
			"## How it works — the mechanism; INCLUDE a ```mermaid diagram of the flow.\n"+
			"## Key files — the files/functions that matter, as a list.\n"+
			"## Decisions — trade-offs taken and why.\n"+
			"## Related — issue keys, other docs.\n\n"+
			"Use type 'feature' for an evergreen feature/area doc, 'decision' for an ADR, 'overview' for a system map, "+
			"'reference' otherwise."),
		mcp.WithString("id", mcp.Description("Document id to update; omit to create")),
		mcp.WithString("title", mcp.Description("Document title")),
		mcp.WithString("body", mcp.Description("Markdown body (use the recommended structure; include a ```mermaid diagram)")),
		mcp.WithString("type", mcp.Description("feature | change | decision | reference | overview (default: change)")),
		mcp.WithString("issue", mcp.Description("Attach to this issue (ticket) id or key — do this for implementation docs")),
		mcp.WithString("project", mcp.Description("Attach to this project (epic) id")),
		mcp.WithString("initiative", mcp.Description("Attach to this initiative id")),
		mcp.WithArray("labels", mcp.Description("Label names (exclusive groups enforced)"),
			mcp.Items(map[string]any{"type": "string"})),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, documentRef(req.GetString("id", "")), issueRef(req.GetString("issue", "")),
			projectRef(req.GetString("project", "")), iniRef(req.GetString("initiative", "")))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		issueID := req.GetString("issue", "")
		if issueID != "" {
			if is, err := d.resolveIssueRef(ctx, wsID, issueID); err == nil {
				issueID = is.ID
			}
		}
		doc := models.Document{
			ID:           req.GetString("id", ""),
			Title:        req.GetString("title", ""),
			BodyMD:       req.GetString("body", ""),
			Type:         req.GetString("type", "change"),
			Author:       "ai",
			ProjectID:    strp(req.GetString("project", "")),
			IssueID:      strp(issueID),
			InitiativeID: strp(req.GetString("initiative", "")),
		}
		saved, err := d.store.SaveDocument(ctx, wsID, doc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if labels := stringSlice(req, "labels"); labels != nil {
			if err := d.store.SetDocumentLabels(ctx, wsID, saved.ID, nil, labels); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			saved, _ = d.store.GetDocument(ctx, wsID, saved.ID)
		}
		// Live: the web Artifacts list refreshes on this without a reload.
		d.svc.Bus.Publish(events.Event{Type: events.DocumentSaved, WorkspaceID: wsID, Actor: saved.Author, Document: &saved})
		return jsonResult(saved)
	})

	// ---- coverage ----
	s.AddTool(mcp.NewTool("list_issues_missing_docs",
		mcp.WithDescription("List completed (Done) issues that have no attached document yet — the gaps in the "+
			"engineering journal, newest first. Write an implementation doc for each with save_document."),
		limitArg(),
		verboseArg(),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsIDs, err := d.scopeAll(ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		issues, err := d.store.IssuesMissingDocs(ctx, wsIDs)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// The gap worth closing is the one just made. Issue numbers are per
		// workspace, so only the update time orders them across workspaces.
		slices.SortFunc(issues, func(a, b models.Issue) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
		return d.issueList(ctx, req, wsIDs, issues, listLimit(req))
	})

	// ---- user ----
	s.AddTool(mcp.NewTool("get_user",
		mcp.WithDescription("Get the current (single) user."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		u, err := d.store.FirstUser(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(u)
	})
}
