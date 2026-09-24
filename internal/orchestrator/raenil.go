package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"raenil/internal/models"
)

// RaenilClient talks to a Raenil server over its HTTP API with a bearer token.
//
// The orchestrator deliberately does not reach into the database directly: it
// may run on a different machine from the tracker, and going through the same
// API the MCP tools use means the record it writes is the record everyone sees.
type RaenilClient struct {
	BaseURL   string
	Token     string
	Workspace string
	HTTP      *http.Client
}

func (c *RaenilClient) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *RaenilClient) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if c.Workspace != "" {
		req.Header.Set("X-Workspace", c.Workspace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("raenil %s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(b))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Health verifies the tracker is reachable before a daemon claims anything.
func (c *RaenilClient) Health(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/api/health", nil, nil)
}

// Issue fetches an issue by key (K-42) or UUID.
func (c *RaenilClient) Issue(ctx context.Context, ref string) (models.Issue, error) {
	var is models.Issue
	return is, c.do(ctx, http.MethodGet, "/api/issues/"+ref, nil, &is)
}

// Criteria reads an issue's done-when checklist.
func (c *RaenilClient) Criteria(ctx context.Context, issueID string) ([]models.Criterion, error) {
	var cs []models.Criterion
	return cs, c.do(ctx, http.MethodGet, "/api/issues/"+issueID+"/criteria", nil, &cs)
}

// TickCriterion marks one criterion, recording what verified it.
//
// This is called the moment a criterion is satisfied, not in a batch at the end:
// a criterion met mid-build gets ticked then, so a dropped run leaves real
// partial progress behind and a live view reflects actual state.
func (c *RaenilClient) TickCriterion(ctx context.Context, criterionID string, done bool, evidenceRef string) error {
	body := map[string]any{"done": done}
	if evidenceRef != "" {
		body["evidenceRef"] = evidenceRef
	}
	return c.do(ctx, http.MethodPatch, "/api/criteria/"+criterionID, body, nil)
}

// SetState moves an issue to a named workflow state.
func (c *RaenilClient) SetState(ctx context.Context, issueID, stateName string) error {
	return c.do(ctx, http.MethodPatch, "/api/issues/"+issueID,
		map[string]any{"stateName": stateName}, nil)
}

// SetDev records the branch an attempt is working on.
func (c *RaenilClient) SetDev(ctx context.Context, issueID, branch, pr string) error {
	// The endpoint's field names are gitBranch/prUrl, not branch/pr — it rejects
	// the whole body otherwise.
	body := map[string]any{"gitBranch": branch, "prUrl": pr}
	return c.do(ctx, http.MethodPatch, "/api/issues/"+issueID+"/dev", body, nil)
}

// LinkCommit records the commit that did the work.
func (c *RaenilClient) LinkCommit(ctx context.Context, issueID, sha, message string) error {
	return c.do(ctx, http.MethodPost, "/api/issues/"+issueID+"/commits",
		map[string]any{"sha": sha, "message": message}, nil)
}

// Comment appends a comment — how a daemon explains itself to a human.
func (c *RaenilClient) Comment(ctx context.Context, issueID, bodyMD string) error {
	return c.do(ctx, http.MethodPost, "/api/issues/"+issueID+"/comments",
		map[string]any{"bodyMd": bodyMD}, nil)
}

// RunRecord is the run as Raenil stores it; only the id is needed back.
type RunRecord struct {
	ID string `json:"id"`
}

// StartRun records that an attempt has begun, so it shows on the ticket while
// it is still running.
func (c *RaenilClient) StartRun(ctx context.Context, s RunStartReq) (RunRecord, error) {
	var out RunRecord
	err := c.do(ctx, http.MethodPost, "/api/runs", s, &out)
	return out, err
}

// RunStartReq is what StartRun records.
type RunStartReq struct {
	IssueID string `json:"issue"`
	AgentID string `json:"agent,omitempty"`
	Kind    string `json:"kind,omitempty"` // work | chat
	Runner  string `json:"runner"`
	Model   string `json:"model"`
	Attempt int    `json:"attempt"`
	Host    string `json:"host"`
}

// Comments lists a ticket's comments, oldest first.
func (c *RaenilClient) Comments(ctx context.Context, issueID string) ([]models.Comment, error) {
	var out []models.Comment
	return out, c.do(ctx, http.MethodGet, "/api/issues/"+issueID+"/comments", nil, &out)
}

// CommentAs posts a comment written by an agent.
func (c *RaenilClient) CommentAs(ctx context.Context, issueID, bodyMD, agentID string) error {
	return c.do(ctx, http.MethodPost, "/api/issues/"+issueID+"/comments",
		map[string]any{"bodyMd": bodyMD, "agentId": agentID}, nil)
}

// Interactions lists what agents asked on a ticket, oldest first.
func (c *RaenilClient) Interactions(ctx context.Context, issueID string) ([]models.Interaction, error) {
	var out []models.Interaction
	return out, c.do(ctx, http.MethodGet, "/api/issues/"+issueID+"/interactions", nil, &out)
}

// AgentSession reads an agent's session on a ticket; ok is false when it has none.
func (c *RaenilClient) AgentSession(ctx context.Context, agentID, issueRef string) (models.AgentSession, bool, error) {
	var out models.AgentSession
	err := c.do(ctx, http.MethodGet, "/api/agents/"+agentID+"/sessions/"+issueRef, nil, &out)
	if err != nil && strings.Contains(err.Error(), "404") {
		return out, false, nil
	}
	return out, err == nil, err
}

// SaveAgentSession records the session a turn ended in.
func (c *RaenilClient) SaveAgentSession(ctx context.Context, agentID, issueRef, sessionID, cwd string) error {
	return c.do(ctx, http.MethodPut, "/api/agents/"+agentID+"/sessions/"+issueRef,
		map[string]any{"sessionId": sessionID, "cwd": cwd}, nil)
}

// RunOutcome is what FinishRun reports.
type RunOutcome struct {
	Status      string         `json:"status"`
	Verdict     string         `json:"verdict,omitempty"`
	SessionID   string         `json:"sessionId,omitempty"`
	ExitCode    *int           `json:"exitCode,omitempty"`
	AgentError  string         `json:"agentError,omitempty"`
	Tokens      map[string]int `json:"tokens"`
	CostUSD     float64        `json:"costUsd"`
	NotionalUSD float64        `json:"notionalCostUsd"`
	Billing     string         `json:"billing,omitempty"`
	DeniedTools []string       `json:"deniedTools,omitempty"`
	LogTail     string         `json:"logTail,omitempty"`
}

// FinishRun closes a run with what the attempt produced.
func (c *RaenilClient) FinishRun(ctx context.Context, runID string, o RunOutcome) error {
	return c.do(ctx, http.MethodPatch, "/api/runs/"+runID, o, nil)
}

// Heartbeat tells Raenil this host is alive and what it can run.
func (c *RaenilClient) Heartbeat(ctx context.Context, host, version string, harnesses []models.HarnessStatus) error {
	return c.do(ctx, http.MethodPost, "/api/hosts/heartbeat",
		map[string]any{"name": host, "version": version, "harnesses": harnesses}, nil)
}

// ClaimedJob is a job handed to this host, with the agent it is for.
type ClaimedJob struct {
	models.Job
	Agent *models.Agent `json:"agent"`
}

// ClaimJob asks for the next job this host can run. ok is false when there is
// none.
func (c *RaenilClient) ClaimJob(ctx context.Context, host string, harnesses []string) (job ClaimedJob, ok bool, err error) {
	err = c.do(ctx, http.MethodPost, "/api/jobs/claim", map[string]any{"host": host, "harnesses": harnesses}, &job)
	return job, err == nil && job.ID != "", err
}

// FinishJob reports how a claimed job ended.
func (c *RaenilClient) FinishJob(ctx context.Context, id, host, status string, result any, errText string) error {
	return c.do(ctx, http.MethodPost, "/api/jobs/"+id+"/finish",
		map[string]any{"host": host, "status": status, "result": result, "error": errText}, nil)
}

// SaveDocument attaches the engineering artifact for an issue.
func (c *RaenilClient) SaveDocument(ctx context.Context, issueID, title, bodyMD, docType string) error {
	if docType == "" {
		docType = "implementation"
	}
	return c.do(ctx, http.MethodPost, "/api/documents", map[string]any{
		"title": title, "bodyMd": bodyMD, "type": docType, "issueId": issueID,
	}, nil)
}

// AddLabel adds a label without disturbing the ones already on the issue.
//
// It reads the current set and sends it back with the new name appended, because
// the update endpoint takes the whole label set. Sending only the new name would
// silently strip an issue's repo/type/domain labels.
func (c *RaenilClient) AddLabel(ctx context.Context, issueID, name string) error {
	is, err := c.Issue(ctx, issueID)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(is.Labels)+1)
	for _, l := range is.Labels {
		if l.Name == name {
			return nil // already there
		}
		names = append(names, l.Name)
	}
	names = append(names, name)
	return c.do(ctx, http.MethodPatch, "/api/issues/"+issueID,
		map[string]any{"labelNames": names}, nil)
}

// States lists the workspace's workflow states.
func (c *RaenilClient) States(ctx context.Context) ([]models.WorkflowState, error) {
	var out []models.WorkflowState
	return out, c.do(ctx, http.MethodGet, "/api/states", nil, &out)
}

// IssuesInState lists issues sitting in a named state, oldest position first.
// The listing endpoint filters by state id, so the name is resolved first.
func (c *RaenilClient) IssuesInState(ctx context.Context, stateName string, limit int) ([]models.Issue, error) {
	states, err := c.States(ctx)
	if err != nil {
		return nil, err
	}
	var id string
	for _, s := range states {
		if strings.EqualFold(s.Name, stateName) {
			id = s.ID
			break
		}
	}
	if id == "" {
		return nil, fmt.Errorf("no workflow state named %q", stateName)
	}
	path := "/api/issues?state=" + url.QueryEscape(id)
	if limit > 0 {
		path += "&limit=" + strconv.Itoa(limit)
	}
	var out []models.Issue
	return out, c.do(ctx, http.MethodGet, path, nil, &out)
}

// AddCriterion appends one typed criterion to an issue.
func (c *RaenilClient) AddCriterion(ctx context.Context, issueRef string, body, kind string, check json.RawMessage) error {
	payload := map[string]any{"body": body, "kind": kind}
	if len(check) > 0 {
		payload["checkSpec"] = check
	}
	return c.do(ctx, http.MethodPost, "/api/issues/"+issueRef+"/criteria", payload, nil)
}

// DeleteCriterion removes one criterion by its own id.
func (c *RaenilClient) DeleteCriterion(ctx context.Context, criterionID string) error {
	return c.do(ctx, http.MethodDelete, "/api/criteria/"+criterionID, nil, nil)
}

// ReplaceCriteria swaps an issue's whole checklist for a new one.
//
// The existing list is removed only after every new item has been accepted, so a
// rejected draft cannot leave a ticket with no acceptance criteria at all.
func (c *RaenilClient) ReplaceCriteria(ctx context.Context, issueRef string, items []ProposedCriterion) error {
	issue, err := c.Issue(ctx, issueRef)
	if err != nil {
		return err
	}
	old, err := c.Criteria(ctx, issue.ID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := c.AddCriterion(ctx, issue.ID, it.Text, it.Kind, it.Check); err != nil {
			return fmt.Errorf("add %q: %w", it.Text, err)
		}
	}
	for _, o := range old {
		if err := c.DeleteCriterion(ctx, o.ID); err != nil {
			return fmt.Errorf("remove old criterion %q: %w", o.Body, err)
		}
	}
	return nil
}

// StateName resolves a workflow state id to its name.
func (c *RaenilClient) StateName(ctx context.Context, stateID string) (string, error) {
	states, err := c.States(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range states {
		if s.ID == stateID {
			return s.Name, nil
		}
	}
	return "", fmt.Errorf("no workflow state with id %s", stateID)
}

// Workspaces lists every workspace this token can reach. It needs no workspace
// header itself, which is what makes it usable to resolve one.
func (c *RaenilClient) Workspaces(ctx context.Context) ([]models.Workspace, error) {
	var out []models.Workspace
	return out, c.do(ctx, http.MethodGet, "/api/workspaces", nil, &out)
}

// Scoped returns a client pinned to the workspace that owns a ticket.
//
// A token spanning several workspaces is refused without an X-Workspace header,
// so the workspace has to come from somewhere. It comes from the ticket key:
// prefixes are unique per workspace, so API-42 names its own workspace and a
// caller never has to say twice which project they meant.
func (c *RaenilClient) Scoped(ctx context.Context, ref string) (*RaenilClient, error) {
	if c.Workspace != "" {
		return c, nil
	}
	prefix, _, ok := splitIssueKey(ref)
	if !ok {
		return nil, fmt.Errorf("cannot tell which workspace %q belongs to: "+
			"pass a ticket key like API-42, or set RAENIL_WORKSPACE", ref)
	}
	spaces, err := c.Workspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace for %s: %w", ref, err)
	}
	var known []string
	for _, w := range spaces {
		if strings.EqualFold(w.KeyPrefix, prefix) {
			scoped := *c
			scoped.Workspace = w.Slug
			return &scoped, nil
		}
		known = append(known, w.KeyPrefix)
	}
	return nil, fmt.Errorf("no workspace uses the key prefix %q (have: %s)",
		prefix, strings.Join(known, ", "))
}

// splitIssueKey splits PP-175 into its prefix and number.
func splitIssueKey(ref string) (prefix, number string, ok bool) {
	i := strings.LastIndexByte(ref, '-')
	if i <= 0 || i == len(ref)-1 {
		return "", "", false
	}
	prefix, number = ref[:i], ref[i+1:]
	for _, r := range number {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	return prefix, number, true
}

// LabelGroups maps each label group's name to its id.
func (c *RaenilClient) LabelGroups(ctx context.Context) (map[string]string, error) {
	var groups []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/label-groups", nil, &groups); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(groups))
	for _, g := range groups {
		out[strings.ToLower(g.Name)] = g.ID
	}
	return out, nil
}
