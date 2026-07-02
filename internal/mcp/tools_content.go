package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"kanri/internal/auth"
	"kanri/internal/models"
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
		mcp.WithDescription("List markdown documents, optionally within a project."),
		mcp.WithString("project", mcp.Description("Project id filter")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		docs, err := d.store.ListDocuments(ctx, req.GetString("project", ""))
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
		mcp.WithDescription("Create (omit id) or update a markdown document. May contain ```mermaid blocks."),
		mcp.WithString("id", mcp.Description("Document id to update; omit to create")),
		mcp.WithString("title", mcp.Description("Document title")),
		mcp.WithString("body", mcp.Description("Markdown body")),
		mcp.WithString("project", mcp.Description("Owning project id")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		doc := models.Document{
			ID:        req.GetString("id", ""),
			Title:     req.GetString("title", ""),
			BodyMD:    req.GetString("body", ""),
			ProjectID: strp(req.GetString("project", "")),
		}
		saved, err := d.store.SaveDocument(ctx, doc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(saved)
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
