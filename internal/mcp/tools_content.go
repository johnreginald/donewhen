package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"raenil/internal/auth"
	"raenil/internal/models"
	"raenil/internal/store"
)

func (d *deps) registerContent(s *server.MCPServer) {
	// ---- comments ----
	s.AddTool(mcp.NewTool("list_comments",
		mcp.WithDescription("List comments on an issue."),
		mcp.WithString("issueId", mcp.Required(), mcp.Description("Issue id or key")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref, err := req.RequireString("issueId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		comments, err := d.store.ListComments(ctx, is.ID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(comments)
	})

	s.AddTool(mcp.NewTool("save_comment",
		mcp.WithDescription("Add a comment to an issue."),
		mcp.WithString("issueId", mcp.Required(), mcp.Description("Issue id or key")),
		mcp.WithString("body", mcp.Required(), mcp.Description("Markdown comment body")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref, err := req.RequireString("issueId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		body, err := req.RequireString("body")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		c, err := d.svc.AddComment(ctx, is.ID, body, auth.ActorAI)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(c)
	})

	// ---- documents ----
	s.AddTool(mcp.NewTool("list_documents",
		mcp.WithDescription("List markdown documents, optionally attached to a project, issue, or initiative."),
		mcp.WithString("project", mcp.Description("Project id filter")),
		mcp.WithString("issue", mcp.Description("Issue id filter")),
		mcp.WithString("initiative", mcp.Description("Initiative id filter")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		docs, err := d.store.ListDocuments(ctx, store.DocFilter{
			ProjectID:    req.GetString("project", ""),
			IssueID:      req.GetString("issue", ""),
			InitiativeID: req.GetString("initiative", ""),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(docs)
	})

	s.AddTool(mcp.NewTool("get_document",
		mcp.WithDescription("Get a document by id."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Document id")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		doc, err := d.store.GetDocument(ctx, id)
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
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		issueID := req.GetString("issue", "")
		if issueID != "" {
			if is, err := d.resolveIssueRef(ctx, issueID); err == nil {
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
		saved, err := d.store.SaveDocument(ctx, doc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if labels := stringSlice(req, "labels"); labels != nil {
			if err := d.store.SetDocumentLabels(ctx, saved.ID, nil, labels); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			saved, _ = d.store.GetDocument(ctx, saved.ID)
		}
		return jsonResult(saved)
	})

	// ---- coverage ----
	s.AddTool(mcp.NewTool("list_issues_missing_docs",
		mcp.WithDescription("List completed (Done) issues that have no attached document yet — the gaps in the "+
			"engineering journal. Write an implementation doc for each with save_document."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		issues, err := d.store.IssuesMissingDocs(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(issues)
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
