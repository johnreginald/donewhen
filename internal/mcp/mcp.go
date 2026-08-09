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
	mgr   *auth.Manager
}

// wsArg is offered by every tool as an override. It is almost never needed:
// a read spans every workspace the account belongs to, and a call that names an
// existing issue, epic, initiative or document has its workspace derived from
// that entity. Only creating something with no parent to inherit from is
// genuinely ambiguous.
func wsArg() mcp.ToolOption {
	return mcp.WithString("workspace",
		mcp.Description("Workspace slug, id or key prefix (e.g. 'lumos'). Optional — reads span all your workspaces and writes infer it from the issue/epic you name. Needed only when creating something with no parent."))
}

// caller returns the account behind this tool call. The stdio transport carries
// no HTTP auth, so it falls back to the account that owns the installation.
func (d *deps) caller(ctx context.Context) (models.User, error) {
	if u, ok := auth.UserFrom(ctx); ok {
		return u, nil
	}
	return d.store.FirstUser(ctx)
}

// reachable lists the workspaces this call may touch: the account's
// memberships, narrowed to the token's pin when it has one.
func (d *deps) reachable(ctx context.Context) ([]models.Membership, error) {
	user, err := d.caller(ctx)
	if err != nil {
		return nil, err
	}
	all, err := d.store.ListMemberships(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if pin, pinned := auth.TokenPinFrom(ctx); pinned {
		kept := all[:0]
		for _, m := range all {
			if m.ID == pin {
				kept = append(kept, m)
			}
		}
		return kept, nil
	}
	return all, nil
}

// named resolves an explicit `workspace` argument, proving membership (and
// refusing to let a pinned token point elsewhere). Returns "" when absent.
func (d *deps) named(ctx context.Context, req mcp.CallToolRequest) (string, error) {
	ref := req.GetString("workspace", "")
	if ref == "" {
		return "", nil
	}
	user, err := d.caller(ctx)
	if err != nil {
		return "", err
	}
	wsp, _, err := d.mgr.ResolveWorkspace(ctx, user, ref)
	if err != nil {
		return "", err
	}
	return wsp.ID, nil
}

// scopeAll is the read scope: every workspace this call can reach. Listing
// across them shows the account nothing it could not see by asking for each in
// turn, so there is no reason to make it ask.
func (d *deps) scopeAll(ctx context.Context, req mcp.CallToolRequest) ([]string, error) {
	if one, err := d.named(ctx, req); err != nil {
		return nil, err
	} else if one != "" {
		return []string{one}, nil
	}
	ms, err := d.reachable(ctx)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, auth.ErrNoWorkspace
	}
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	return ids, nil
}

// ref is an entity a call names, from which its workspace can be derived.
// table "" means "an issue", which accepts a uuid or a human key.
type ref struct{ table, id string }

func issueRef(id string) ref    { return ref{"", id} }
func projectRef(id string) ref  { return ref{"projects", id} }
func iniRef(id string) ref      { return ref{"initiatives", id} }
func documentRef(id string) ref { return ref{"documents", id} }
func stateRef(id string) ref    { return ref{"workflow_states", id} }

// scopeOne is the write scope: the single workspace a call acts on.
//
// Order: an explicit argument, then the token's pin, then the workspace of the
// first entity the call names — a lookup, not a guess, since issue keys and row
// ids are unique. Membership is checked on whatever comes back, so deriving can
// never reach a workspace the account is not in. Only a call that names nothing
// falls through to "your sole workspace, or say which".
func (d *deps) scopeOne(ctx context.Context, req mcp.CallToolRequest, refs ...ref) (string, error) {
	if one, err := d.named(ctx, req); err != nil {
		return "", err
	} else if one != "" {
		return one, nil
	}
	if pin, pinned := auth.TokenPinFrom(ctx); pinned {
		return pin, nil
	}

	ms, err := d.reachable(ctx)
	if err != nil {
		return "", err
	}
	member := func(id string) bool {
		for _, m := range ms {
			if m.ID == id {
				return true
			}
		}
		return false
	}

	for _, r := range refs {
		if r.id == "" {
			continue
		}
		var wsID string
		var err error
		if r.table == "" {
			wsID, err = d.store.WorkspaceOfIssueRef(ctx, r.id)
		} else {
			wsID, err = d.store.WorkspaceOf(ctx, r.table, r.id)
		}
		if err != nil {
			continue // unknown reference: let the tool itself report it
		}
		if !member(wsID) {
			return "", store.ErrNotMember
		}
		return wsID, nil
	}

	switch len(ms) {
	case 0:
		return "", auth.ErrNoWorkspace
	case 1:
		return ms[0].ID, nil
	default:
		return "", auth.ErrAmbiguousWorkspace
	}
}

func buildServer(d *deps) *server.MCPServer {
	s := server.NewMCPServer("raenil", version,
		server.WithToolCapabilities(true),
		server.WithInstructions(
			"Raenil issue tracker. Continuous-flow Kanban: Triage → Backlog → Aligning → "+
				"Ready → In Progress → In Review → Done → Canceled. Hierarchy is "+
				"Workspace → Project (initiative) → Epic (project) → Issue. Listing tools "+
				"span every workspace you can reach (each row carries workspaceId), and a "+
				"call that names an issue/epic/initiative infers the workspace from it — so "+
				"you rarely need the 'workspace' argument. Pass it when creating something "+
				"with no parent. list_workspaces shows what you can reach. "+
				"Use save_issue to create/move issues (pass 'state' as a status name). "+
				"Backend-labeled issues should include a ```mermaid diagram in the description.",
		),
	)
	d.register(s)
	return s
}

// NewHandler builds the bearer-authed MCP HTTP handler mounted at /mcp.
func NewHandler(svc *service.Service, st *store.Store, cfg config.Config) http.Handler {
	d := &deps{svc: svc, store: st, cfg: cfg, mgr: auth.NewManager(st, cfg.SecureCookies())}
	httpSrv := server.NewStreamableHTTPServer(buildServer(d))
	return requireBearer(st, httpSrv)
}

// ServeStdio runs the MCP server over stdio (local fallback transport).
func ServeStdio(svc *service.Service, st *store.Store, cfg config.Config) error {
	d := &deps{svc: svc, store: st, cfg: cfg, mgr: auth.NewManager(st, cfg.SecureCookies())}
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
		if _, _, err := st.LookupAPIToken(r.Context(), auth.HashToken(tok)); err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		// auth.Middleware has already put the user and any workspace pin on the
		// context; tools read them through d.ws.
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

// resolveIssueRef accepts a uuid or a human key (K-42) and returns the issue
// from within one workspace.
func (d *deps) resolveIssueRef(ctx context.Context, wsID, ref string) (models.Issue, error) {
	if !isUUID(ref) {
		// A human key. Falling through to GetIssue would compare it against a
		// uuid column and surface a raw Postgres type error instead of a clean
		// "not found" — which is what a key from another workspace looks like.
		return d.store.GetIssueByKey(ctx, wsID, ref)
	}
	return d.store.GetIssue(ctx, wsID, ref)
}

// isUUID reports the canonical 8-4-4-4-12 shape, which is how an id is told
// apart from an issue key like "ZLA-288".
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
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
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		wsIDs, err := d.scopeAll(ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		f := store.IssueFilter{
			WorkspaceIDs: wsIDs,
			StateName:    req.GetString("state", ""),
			ProjectID:    req.GetString("project", ""),
			InitiativeID: req.GetString("initiative", ""),
			LabelID:      req.GetString("label", ""),
			ParentKey:    req.GetString("parent", ""),
			Query:        req.GetString("query", ""),
			Limit:        req.GetInt("limit", 0),
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
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, issueRef(key))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, key)
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
		wsArg(),
	), d.handleSaveIssue)

	// ---- delete_issue ----
	s.AddTool(mcp.NewTool("delete_issue",
		mcp.WithDescription("Delete an issue by id or key."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Issue id or key")),
		wsArg(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ref2, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		wsID, err := d.scopeOne(ctx, req, issueRef(ref2))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		is, err := d.resolveIssueRef(ctx, wsID, ref2)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := d.svc.DeleteIssue(ctx, wsID, is.ID, auth.ActorAI); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]string{"status": "deleted", "key": is.Key})
	})

	// ---- list_workspaces ----
	s.AddTool(mcp.NewTool("list_workspaces",
		mcp.WithDescription("List the workspaces you can act on. Every other tool operates on exactly one of these; "+
			"pass its slug as 'workspace' unless your token is pinned."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		items, err := d.reachable(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(items)
	})

	// ---- save_workspace ----
	s.AddTool(mcp.NewTool("save_workspace",
		mcp.WithDescription("Create a workspace (omit id) or rename/re-prefix the active one (pass id)."),
		mcp.WithString("id", mcp.Description("Workspace id to update; omit to create")),
		mcp.WithString("name", mcp.Description("Display name")),
		mcp.WithString("slug", mcp.Description("URL-safe handle; derived from the name when omitted")),
		mcp.WithString("keyPrefix", mcp.Description("Issue key prefix for NEW issues, e.g. 'LUM'")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user, ok := auth.UserFrom(ctx)
		if !ok {
			u, err := d.store.FirstUser(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			user = u
		}
		id := req.GetString("id", "")
		prefix := req.GetString("keyPrefix", "")
		if prefix != "" {
			if err := store.ValidatePrefix(prefix, d.store.ReservedPrefix()); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
		if id == "" {
			name := req.GetString("name", "")
			if name == "" || prefix == "" {
				return mcp.NewToolResultError("name and keyPrefix are required to create a workspace"), nil
			}
			ws, err := d.store.CreateWorkspace(ctx, name, req.GetString("slug", ""), prefix, user.ID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(ws)
		}
		if _, err := d.store.RoleIn(ctx, id, user.ID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		ws, err := d.store.UpdateWorkspace(ctx, id,
			strp(req.GetString("name", "")), strp(req.GetString("slug", "")), strp(prefix))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(ws)
	})

	d.registerMeta(s)
	d.registerContent(s)
	d.registerDev(s)
}

func (d *deps) handleSaveIssue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := req.GetString("id", "")
	labels := stringSlice(req, "labels")
	// Updating? the issue says which workspace. Creating into an epic? the epic
	// says. Creating bare is the one case with nothing to infer from.
	wsID, err := d.scopeOne(ctx, req, issueRef(id), projectRef(req.GetString("project", "")))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if id == "" {
		// create
		is, err := d.svc.CreateIssue(ctx, wsID, store.IssueInput{
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
	existing, err := d.resolveIssueRef(ctx, wsID, id)
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
	is, err := d.svc.UpdateIssue(ctx, wsID, existing.ID, p, auth.ActorAI)
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

func (d *deps) stateID(ctx context.Context, wsID, name string) (string, error) {
	states, err := d.store.ListStates(ctx, wsID)
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
