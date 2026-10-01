package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"raenil/internal/auth"
	"raenil/internal/config"
	"raenil/internal/db"
	"raenil/internal/store"
)

// callTool runs one tools/call through the real bearer + auth middleware and
// stateless streamable HTTP transport, returning (isError, text).
func callTool(t *testing.T, h http.Handler, token, name string, args map[string]any) (bool, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	raw := rec.Body.String()
	// The transport may answer as an SSE frame; take the JSON payload.
	if i := strings.Index(raw, "{"); i >= 0 {
		raw = raw[i:]
	}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&resp); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if resp.Error != nil {
		t.Fatalf("rpc error: %s", resp.Error.Message)
	}
	text := ""
	if len(resp.Result.Content) > 0 {
		text = resp.Result.Content[0].Text
	}
	return resp.Result.IsError, text
}

func TestMCPLinkURLsAreHTTPOnly(t *testing.T) {
	dsn := os.Getenv("RAENIL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set RAENIL_TEST_DATABASE_URL to run MCP tests")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	st := store.New(pool, "K")

	n := time.Now().UnixNano()
	u, err := st.CreateUser(ctx, fmt.Sprintf("mcp%d@example.test", n), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID)
	suffix := fmt.Sprintf("%d", n%1000000)
	w, err := st.CreateWorkspace(ctx, "MCP "+suffix, "mcp-"+suffix, "M"+suffix, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, w.ID)
	is, err := st.CreateIssue(ctx, w.ID, store.IssueInput{Title: "x", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("tok-%d", n)
	if _, err := st.CreateAPIToken(ctx, u.ID, "t", auth.HashToken(token), nil); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{}
	d := &deps{store: st, cfg: cfg, mgr: auth.NewManager(st, false)}
	srv := server.NewStreamableHTTPServer(buildServer(d), server.WithStateLess(true))
	h := d.mgr.Middleware(requireBearer(st, srv))

	for _, bad := range []string{"javascript:alert(1)", "data:text/html,x", "file:///etc/passwd"} {
		isErr, text := callTool(t, h, token, "set_issue_dev", map[string]any{"issue": is.Key, "prUrl": bad})
		if !isErr || !strings.Contains(text, "invalid_url") {
			t.Errorf("set_issue_dev %q: isError=%v %s", bad, isErr, text)
		}
		isErr, text = callTool(t, h, token, "link_commit", map[string]any{"issue": is.Key, "sha": "abc1234", "url": bad})
		if !isErr || !strings.Contains(text, "invalid_url") {
			t.Errorf("link_commit %q: isError=%v %s", bad, isErr, text)
		}
	}
	isErr, text := callTool(t, h, token, "set_issue_dev", map[string]any{"issue": is.Key, "prUrl": "https://github.com/x/y/pull/1"})
	if isErr {
		t.Errorf("valid prUrl rejected: %s", text)
	}
	isErr, text = callTool(t, h, token, "link_commit", map[string]any{"issue": is.Key, "sha": "abc1234", "url": "https://github.com/x/y/commit/abc1234"})
	if isErr {
		t.Errorf("valid commit url rejected: %s", text)
	}
}
