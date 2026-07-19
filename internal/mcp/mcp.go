// Package mcp exposes Raenil over the Model Context Protocol using a tool
// surface that mirrors Linear's verbs (save_issue, list_issues, ...), served
// over Streamable HTTP and authenticated with a bearer API token.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"raenil/internal/auth"
	"raenil/internal/config"
	"raenil/internal/models"
	"raenil/internal/service"
	"raenil/internal/store"
)

const version = "1.0.0"

type deps struct {
	svc   *service.Service
	store *store.Store
	cfg   config.Config
}

func buildServer(d *deps) *server.MCPServer {
	s := server.NewMCPServer("raenil", version,
		server.WithToolCapabilities(true),
		server.WithInstructions(
			"Raenil issue tracker. Continuous-flow Kanban: Triage → Backlog → Aligning → "+
				"Ready → In Progress → In Review → Done → Canceled. Hierarchy is "+
				"Initiatives → Projects → Issues. Use save_issue to create/move issues "+
				"(pass 'state' as a status name). Backend-labeled issues should include a "+
				"```mermaid diagram in the description.",
		),
	)
	d.register(s)
	return s
}

// NewHandler builds the bearer-authed MCP HTTP handler mounted at /mcp.
func NewHandler(svc *service.Service, st *store.Store, cfg config.Config) http.Handler {
	d := &deps{svc: svc, store: st, cfg: cfg}
	httpSrv := server.NewStreamableHTTPServer(buildServer(d))
	return requireBearer(st, httpSrv)
}

// ServeStdio runs the MCP server over stdio (local fallback transport).
func ServeStdio(svc *service.Service, st *store.Store, cfg config.Config) error {
	d := &deps{svc: svc, store: st, cfg: cfg}
	return server.ServeStdio(buildServer(d))
}

// requireBearer rejects MCP requests lacking a valid API token.
func requireBearer(st *store.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
			return
		}
		tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if _, err := st.LookupAPIToken(r.Context(), auth.HashToken(tok)); err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

func stringSlice(req mcp.CallToolRequest, key string) []string {
	raw, ok := req.GetArguments()[key]
	if !ok || raw == nil {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// criterionItem is one done-when line parsed from a set_criteria request.
type criterionItem struct {
	text string
	done bool
}

// criteriaItems parses the "items" arg, accepting either objects {text, done}
// or bare strings (treated as not-done). Blank text is skipped.
func criteriaItems(req mcp.CallToolRequest) []criterionItem {
	raw, ok := req.GetArguments()["items"]
	if !ok || raw == nil {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]criterionItem, 0, len(arr))
	for _, v := range arr {
		switch t := v.(type) {
		case string:
			if t != "" {
				out = append(out, criterionItem{text: t})
			}
		case map[string]any:
			text, _ := t["text"].(string)
			if text == "" {
				continue
			}
			done, _ := t["done"].(bool)
			out = append(out, criterionItem{text: text, done: done})
		}
	}
	return out
}

// resolveIssueRef accepts a uuid or a human key (K-42) and returns the issue.
func (d *deps) resolveIssueRef(ctx context.Context, ref string) (models.Issue, error) {
	if strings.Contains(ref, "-") && !strings.Contains(ref, "0000-") {
		if is, err := d.store.GetIssueByKey(ctx, ref); err == nil {
			return is, nil
		}
	}
	return d.store.GetIssue(ctx, ref)
}

func (d *deps) register(s *server.MCPServer) {
	// ---- list_issues ----
	s.AddTool(mcp.NewTool("list_issues",
		mcp.WithDescription("List issues, optionally filtered by state name, project (epic) id, initiative id, label id, parent issue key, and/or a text query."),
		mcp.WithString("state", mcp.Description("Workflow state name, e.g. 'In Review'")),
		mcp.WithString("project", mcp.Description("Project (epic) id — issues in this epic")),
		mcp.WithString("initiative", mcp.Description("Initiative id — all issues whose epic belongs to this initiative")),
		mcp.WithString("label", mcp.Description("Label id — issues carrying this label")),
		mcp.WithString("parent", mcp.Description("Parent issue key (e.g. R-8) — its sub-issues")),
		mcp.WithString("query", mcp.Description("Text search over title/key")),
		mcp.WithNumber("limit", mcp.Description("Max results")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		f := store.IssueFilter{
			ProjectID:    req.GetString("project", ""),
			InitiativeID: req.GetString("initiative", ""),
			LabelID:      req.GetString("label", ""),
			ParentKey:    req.GetString("parent", ""),
			Query:        req.GetString("query", ""),
			Limit:        req.GetInt("limit", 0),
		}
		if name := req.GetString("state", ""); name != "" {
			if id, err := d.stateID(ctx, name); err == nil {
				f.StateID = id
			}
		}
		issues, err := d.store.ListIssues(ctx, f)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(issues)
	})

	// ---- get_issue ----
	s.AddTool(mcp.NewTool("get_issue",
		mcp.WithDescription("Get a single issue by id or key (e.g. K-42)."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Issue id or key")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(is)
	})

	// ---- save_issue (create or update) ----
	s.AddTool(mcp.NewTool("save_issue",
		mcp.WithDescription("Create a new issue (omit id) or update an existing one (pass id/key). "+
			"Set 'state' to a status name to move it. Backend issues should include a ```mermaid diagram in description."),
		mcp.WithString("id", mcp.Description("Issue id or key to update; omit to create")),
		mcp.WithString("title", mcp.Description("Issue title")),
		mcp.WithString("description", mcp.Description("Markdown description (may contain ```mermaid)")),
		mcp.WithString("state", mcp.Description("Workflow state name, e.g. 'Ready'")),
		mcp.WithString("project", mcp.Description("Project id")),
		mcp.WithNumber("priority", mcp.Description("0 none, 1 urgent, 2 high, 3 medium, 4 low")),
		mcp.WithArray("labels", mcp.Description("Label names (exclusive groups enforced)"),
			mcp.Items(map[string]any{"type": "string"})),
	), d.handleSaveIssue)

	// ---- delete_issue ----
	s.AddTool(mcp.NewTool("delete_issue",
		mcp.WithDescription("Delete an issue by id or key."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Issue id or key")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := d.svc.DeleteIssue(ctx, is.ID, auth.ActorAI); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]string{"status": "deleted", "key": is.Key})
	})

	d.registerMeta(s)
	d.registerContent(s)
	d.registerDev(s)
}

func (d *deps) handleSaveIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := req.GetString("id", "")
	labels := stringSlice(req, "labels")

	if id == "" {
		// create
		is, err := d.svc.CreateIssue(ctx, store.IssueInput{
			Title:         req.GetString("title", "Untitled"),
			DescriptionMD: req.GetString("description", ""),
			StateName:     req.GetString("state", ""),
			ProjectID:     strp(req.GetString("project", "")),
			Priority:      req.GetInt("priority", 0),
			LabelNames:    labels,
		}, auth.ActorAI)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(is)
	}

	// update
	existing, err := d.resolveIssueRef(ctx, id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p := store.IssuePatch{}
	args := req.GetArguments()
	if v, ok := args["title"].(string); ok {
		p.Title = &v
	}
	if v, ok := args["description"].(string); ok {
		p.DescriptionMD = &v
	}
	if v, ok := args["state"].(string); ok && v != "" {
		p.StateName = &v
	}
	if v, ok := args["project"].(string); ok {
		p.SetProject = true
		p.ProjectID = strp(v)
	}
	if _, ok := args["priority"]; ok {
		pr := req.GetInt("priority", 0)
		p.Priority = &pr
	}
	if _, ok := args["labels"]; ok {
		p.ReplaceLabels = true
		p.LabelNames = labels
	}
	is, err := d.svc.UpdateIssue(ctx, existing.ID, p, auth.ActorAI)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(is)
}

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (d *deps) stateID(ctx context.Context, name string) (string, error) {
	states, err := d.store.ListStates(ctx)
	if err != nil {
		return "", err
	}
	for _, st := range states {
		if strings.EqualFold(st.Name, name) || st.ID == name {
			return st.ID, nil
		}
	}
	return "", store.ErrNotFound
}
