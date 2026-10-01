package mcp

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	gomcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/johnreginald/donewhen/internal/store"
)

// PP-236 part 2 / PP-195 part 3: a tool that hits a database error answers
// `internal error` and nothing else; validation errors keep their message.
func TestToolErrHidesDatabaseErrors(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "22P02", Message: `invalid input syntax for type uuid: "x"`}
	for name, err := range map[string]error{
		"raw pg":     pgErr,
		"wrapped pg": fmt.Errorf("select issues: %w", pgErr),
		"plain":      errors.New("connection refused to 10.0.0.5:5432"),
	} {
		res := toolErr(err)
		if !res.IsError {
			t.Fatalf("%s: not an error result", name)
		}
		if got := resultErrText(t, res); got != "internal error" {
			t.Errorf("%s: text = %q, want %q", name, got, "internal error")
		}
	}

	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"invalid":  {fmt.Errorf("%w: title is required", store.ErrInvalid), "title is required"},
		"conflict": {fmt.Errorf("%w: key taken", store.ErrConflict), "key taken"},
	} {
		if got := resultErrText(t, toolErr(tc.err)); got != tc.want {
			t.Errorf("%s: text = %q, want %q", name, got, tc.want)
		}
	}
}

// End to end through a real tool: Postgres rejects the malformed project id, and
// the reply carries no SQL text.
func TestToolDatabaseErrorIsGeneric(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	a := e.workspace(uid)
	ctx := e.ctxFor(uid, a.ID)

	out, isErr := e.call(ctx, "get_project", map[string]any{"id": "not-a-uuid"})
	if !isErr {
		t.Fatalf("expected an error, got %s", out)
	}
	if !strings.Contains(out, "internal error") {
		t.Fatalf("reply = %s, want internal error", out)
	}
	for _, leak := range []string{"uuid", "SQLSTATE", "invalid input", "syntax"} {
		if strings.Contains(out, leak) {
			t.Fatalf("reply leaks %q: %s", leak, out)
		}
	}
}

func resultErrText(t *testing.T, res *gomcp.CallToolResult) string {
	t.Helper()
	return res.Content[0].(gomcp.TextContent).Text
}
