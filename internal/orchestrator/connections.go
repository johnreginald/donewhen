package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Connections is where this Mac keeps the subscription logins Raenil's agents
// run on — Paperclip's "My Claude subscription", kept on the machine that
// runs the agents rather than on the server, which never needs the token.
//
//	<dir>/claude/oauth-token   the long-lived token from `claude setup-token`
//	<dir>/codex/               a CODEX_HOME holding Codex's ChatGPT login
//
// Both are private to the user (0700 dirs, 0600 files).
type Connections struct {
	Dir string
}

// DefaultConnections is ~/.raenil/connections.
func DefaultConnections() Connections {
	home, _ := os.UserHomeDir()
	return Connections{Dir: filepath.Join(home, ".raenil", "connections")}
}

// claudeTokenShape is what `claude setup-token` prints: a subscription OAuth
// token, never an API key.
var claudeTokenShape = regexp.MustCompile(`^sk-ant-oat\d+-[A-Za-z0-9_\-]{20,}$`)

func (c Connections) claudeTokenPath() string { return filepath.Join(c.Dir, "claude", "oauth-token") }

// ClaudeToken reads the stored token, if there is one.
func (c Connections) ClaudeToken() (string, bool) {
	b, err := os.ReadFile(c.claudeTokenPath())
	if err != nil {
		return "", false
	}
	tok := strings.TrimSpace(string(b))
	return tok, tok != ""
}

// SaveClaudeToken stores a setup-token. An API key is refused: Claude runs on
// the subscription.
func (c Connections) SaveClaudeToken(tok string) error {
	tok = strings.TrimSpace(tok)
	if strings.HasPrefix(tok, "sk-ant-api") {
		return errors.New("that is an API key; Raenil runs Claude on your subscription — paste the token `claude setup-token` printed")
	}
	if !claudeTokenShape.MatchString(tok) {
		return errors.New("that does not look like a token from `claude setup-token` (sk-ant-oat…)")
	}
	dir := filepath.Dir(c.claudeTokenPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(c.claudeTokenPath(), []byte(tok+"\n"), 0o600)
}

// ClaudeConfigDir is the config directory Raenil's Claude runs share: apart
// from the user's ~/.claude, kept so sessions can be resumed.
func (c Connections) ClaudeConfigDir() string { return filepath.Join(c.Dir, "claude", "config") }

// CodexHome is the CODEX_HOME that holds Codex's login for Raenil.
func (c Connections) CodexHome() string { return filepath.Join(c.Dir, "codex") }

// CodexConnected reports whether a Codex login has been made there.
func (c Connections) CodexConnected() bool {
	_, err := os.Stat(filepath.Join(c.CodexHome(), "auth.json"))
	return err == nil
}

// PrepareCodexHome makes the directory and tells Codex to keep its login in a
// file there rather than the system keychain, so it stays with this home.
func (c Connections) PrepareCodexHome() error {
	if err := os.MkdirAll(c.CodexHome(), 0o700); err != nil {
		return err
	}
	cfg := filepath.Join(c.CodexHome(), "config.toml")
	if _, err := os.Stat(cfg); err == nil {
		return nil
	}
	return os.WriteFile(cfg, []byte("cli_auth_credentials_store = \"file\"\n"), 0o600)
}

// oauthUsageURL is the endpoint Paperclip checks a subscription token against.
var oauthUsageURL = "https://api.anthropic.com/api/oauth/usage"

// VerifyClaudeToken asks Anthropic whether a subscription token is still
// accepted. Only an explicit refusal is an error: a network hiccup or a moved
// endpoint must not stop agents from running.
func VerifyClaudeToken(ctx context.Context, tok string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oauthUsageURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var body struct {
		Error struct {
			Details struct {
				Code string `json:"error_code"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("the Claude connection was refused (%s): run `orchestrator connect claude` again", resp.Status)
	case resp.StatusCode == http.StatusForbidden && body.Error.Details.Code != "oauth_scope_insufficient":
		return fmt.Errorf("the Claude connection was refused (%s): run `orchestrator connect claude` again", resp.Status)
	}
	// A setup-token is scoped to inference only, so this endpoint answers it
	// with a 403 oauth_scope_insufficient — the token is live, just narrow.
	return nil
}
