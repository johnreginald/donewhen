package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"raenil/internal/models"
)

func (d *deps) registerMeta(s *server.MCPServer) {
	// ---- projects ----
	s.AddTool(mcp.NewTool("list_projects",
		mcp.WithDescription("List projects (epics), optionally within an initiative."),
		mcp.WithString("initiative", mcp.Description("Initiative id filter")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		items, err := d.store.ListProjects(ctx, req.GetString("initiative", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(items)
	})

	s.AddTool(mcp.NewTool("get_project",
		mcp.WithDescription("Get a project by id."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Project id")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		p, err := d.store.GetProject(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(p)
	})

	s.AddTool(mcp.NewTool("save_project",
		mcp.WithDescription("Create (omit id) or update a project (epic)."),
		mcp.WithString("id", mcp.Description("Project id to update; omit to create")),
		mcp.WithString("name", mcp.Description("Project name")),
		mcp.WithString("description", mcp.Description("Markdown description")),
		mcp.WithString("initiative", mcp.Description("Parent initiative id")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p := models.Project{
			ID:            req.GetString("id", ""),
			Name:          req.GetString("name", ""),
			DescriptionMD: req.GetString("description", ""),
			InitiativeID:  strp(req.GetString("initiative", "")),
		}
		saved, err := d.store.SaveProject(ctx, p)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(saved)
	})

	// ---- initiatives ----
	s.AddTool(mcp.NewTool("get_initiative",
		mcp.WithDescription("Get an initiative by id."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Initiative id")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		it, err := d.store.GetInitiative(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(it)
	})

	s.AddTool(mcp.NewTool("list_initiatives",
		mcp.WithDescription("List initiatives (top-level grouping of projects)."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		items, err := d.store.ListInitiatives(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(items)
	})

	s.AddTool(mcp.NewTool("save_initiative",
		mcp.WithDescription("Create (omit id) or update an initiative."),
		mcp.WithString("id", mcp.Description("Initiative id to update; omit to create")),
		mcp.WithString("name", mcp.Description("Initiative name")),
		mcp.WithString("description", mcp.Description("Markdown description")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		i := models.Initiative{
			ID:            req.GetString("id", ""),
			Name:          req.GetString("name", ""),
			DescriptionMD: req.GetString("description", ""),
		}
		saved, err := d.store.SaveInitiative(ctx, i)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(saved)
	})

	// ---- statuses ----
	s.AddTool(mcp.NewTool("list_issue_statuses",
		mcp.WithDescription("List the workflow states (Triage → ... → Done, Canceled)."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		states, err := d.store.ListStates(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(states)
	})

	s.AddTool(mcp.NewTool("get_issue_status",
		mcp.WithDescription("Get a workflow state by id."),
		mcp.WithString("id", mcp.Required(), mcp.Description("State id")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		st, err := d.store.GetState(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(st)
	})

	// ---- labels ----
	s.AddTool(mcp.NewTool("list_issue_labels",
		mcp.WithDescription("List labels and their exclusive groups."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		labels, err := d.store.ListLabels(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		groups, _ := d.store.ListLabelGroups(ctx)
		return jsonResult(map[string]any{"labels": labels, "groups": groups})
	})

	s.AddTool(mcp.NewTool("create_issue_label",
		mcp.WithDescription("Create a label, optionally in an exclusive group (repo/platform/type/domain/triage)."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Label name")),
		mcp.WithString("color", mcp.Description("Hex color")),
		mcp.WithString("group", mcp.Description("Group name, e.g. 'domain'")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		l, err := d.store.CreateLabel(ctx, name, req.GetString("color", ""), req.GetString("group", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(l)
	})
}
