package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/models"
)

func (d *deps) registerMeta(s *server.MCPServer) {
	// ---- projects ----
	s.AddTool(mcp.NewTool("list_projects",
		mcp.WithDescription("List projects (epics), optionally within an initiative. Rows carry no description; "+
			"use get_project for it."),
		mcp.WithString("initiative", mcp.Description("Initiative id filter")),
		mcp.WithString("archived", mcp.Description("'false' (default) hides archived epics, 'true' lists only archived, 'all' both")),
		verboseArg(),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsIDs, err := d.scopeAll(ctx, req)
		if err != nil {
			return toolErr(err), nil
		}
		items, err := d.store.ListProjectsAcross(ctx, wsIDs, req.GetString("initiative", ""), req.GetString("archived", ""))
		if err != nil {
			return toolErr(err), nil
		}
		if req.GetBool("verbose", false) {
			return jsonResult(items)
		}
		return jsonResult(projectRows(items))
	})

	s.AddTool(mcp.NewTool("archive_project",
		mcp.WithDescription("Archive (default) or unarchive a project (epic). Archived epics and their issues "+
			"leave the default lists; only the status changes."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Project id")),
		mcp.WithBoolean("archived", mcp.Description("true (default) archives, false unarchives")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, projectRef(id))
		if err != nil {
			return toolErr(err), nil
		}
		p, err := d.store.ArchiveProject(ctx, wsID, id, req.GetBool("archived", true), auth.ActorAI)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(p)
	})

	s.AddTool(mcp.NewTool("get_project",
		mcp.WithDescription("Get a project by id."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Project id")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, projectRef(id))
		if err != nil {
			return toolErr(err), nil
		}
		p, err := d.store.GetProject(ctx, wsID, id)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(p)
	})

	s.AddTool(mcp.NewTool("save_project",
		mcp.WithDescription("Create (omit id) or update a project (epic)."),
		mcp.WithString("id", mcp.Description("Project id to update; omit to create")),
		mcp.WithString("name", mcp.Description("Project name")),
		mcp.WithString("description", mcp.Description("Markdown description")),
		mcp.WithString("initiative", mcp.Description("Parent initiative id")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, projectRef(req.GetString("id", "")), iniRef(req.GetString("initiative", "")))
		if err != nil {
			return toolErr(err), nil
		}
		p := models.Project{
			ID:            req.GetString("id", ""),
			Name:          req.GetString("name", ""),
			DescriptionMD: req.GetString("description", ""),
			InitiativeID:  strp(req.GetString("initiative", "")),
		}
		saved, err := d.store.SaveProject(ctx, wsID, p)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(saved)
	})

	// ---- initiatives ----
	s.AddTool(mcp.NewTool("get_initiative",
		mcp.WithDescription("Get an initiative by id."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Initiative id")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, iniRef(id))
		if err != nil {
			return toolErr(err), nil
		}
		it, err := d.store.GetInitiative(ctx, wsID, id)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(it)
	})

	s.AddTool(mcp.NewTool("list_initiatives",
		mcp.WithDescription("List initiatives (top-level grouping of projects)."),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsIDs, err := d.scopeAll(ctx, req)
		if err != nil {
			return toolErr(err), nil
		}
		items, err := d.store.ListInitiativesAcross(ctx, wsIDs)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(items)
	})

	s.AddTool(mcp.NewTool("save_initiative",
		mcp.WithDescription("Create (omit id) or update an initiative."),
		mcp.WithString("id", mcp.Description("Initiative id to update; omit to create")),
		mcp.WithString("name", mcp.Description("Initiative name")),
		mcp.WithString("description", mcp.Description("Markdown description")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req, iniRef(req.GetString("id", "")))
		if err != nil {
			return toolErr(err), nil
		}
		i := models.Initiative{
			ID:            req.GetString("id", ""),
			Name:          req.GetString("name", ""),
			DescriptionMD: req.GetString("description", ""),
		}
		saved, err := d.store.SaveInitiative(ctx, wsID, i)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(saved)
	})

	// ---- statuses ----
	s.AddTool(mcp.NewTool("list_issue_statuses",
		mcp.WithDescription("List the workflow states (Triage → ... → Done, Canceled)."),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsIDs, err := d.scopeAll(ctx, req)
		if err != nil {
			return toolErr(err), nil
		}
		states, err := d.store.ListStatesAcross(ctx, wsIDs)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(states)
	})

	s.AddTool(mcp.NewTool("get_issue_status",
		mcp.WithDescription("Get a workflow state by id."),
		mcp.WithString("id", mcp.Required(), mcp.Description("State id")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, stateRef(id))
		if err != nil {
			return toolErr(err), nil
		}
		st, err := d.store.GetState(ctx, wsID, id)
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(st)
	})

	// ---- labels ----
	s.AddTool(mcp.NewTool("list_issue_labels",
		mcp.WithDescription("List label names by group (\"\" = ungrouped), merged across your workspaces; "+
			"an exclusive group allows one label per issue. Pass workspace to see one workspace's labels."),
		verboseArg(),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsIDs, err := d.scopeAll(ctx, req)
		if err != nil {
			return toolErr(err), nil
		}
		labels, err := d.store.ListLabelsAcross(ctx, wsIDs)
		if err != nil {
			return toolErr(err), nil
		}
		groups, _ := d.store.ListLabelGroupsAcross(ctx, wsIDs)
		if req.GetBool("verbose", false) {
			return jsonResult(map[string]any{"labels": labels, "groups": groups})
		}
		return jsonResult(labelCatalog(labels, groups))
	})

	s.AddTool(mcp.NewTool("create_issue_label",
		mcp.WithDescription("Create a label, optionally in an exclusive group (repo/platform/type/domain/triage)."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Label name")),
		mcp.WithString("color", mcp.Description("Hex color")),
		mcp.WithString("group", mcp.Description("Group name, e.g. 'domain'")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsID, err := d.scopeOne(ctx, req)
		if err != nil {
			return toolErr(err), nil
		}
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		l, err := d.store.CreateLabel(ctx, wsID, name, req.GetString("color", ""), req.GetString("group", ""))
		if err != nil {
			return toolErr(err), nil
		}
		return jsonResult(l)
	})
}
