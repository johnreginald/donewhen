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
	if out == nil {
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
