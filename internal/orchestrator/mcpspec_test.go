package orchestrator

import (
	"os"
	"strings"
	"testing"
)

func TestCodexMCPArgsKeepTheTokenOffTheCommandLine(t *testing.T) {
	m := RaenilMCP("https://raenil.example", "raenil_secret", "ag-1", "dev", "ask_user")
	args := strings.Join(codexMCPArgs(*m, "RAENIL_MCP_TOKEN"), " ")
	if strings.Contains(args, "raenil_secret") {
		t.Errorf("token on the command line: %s", args)
	}
	for _, want := range []string{
		`mcp_servers.raenil.url="https://raenil.example/mcp"`,
		`mcp_servers.raenil.bearer_token_env_var="RAENIL_MCP_TOKEN"`,
		`"X-Raenil-Tools"="agent"`, `"X-Raenil-Agent"="ag-1"`, `"X-Raenil-Workspace"="dev"`,
		`mcp_servers.raenil.tools.ask_user.approval_mode="approve"`,
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args lack %s:\n%s", want, args)
		}
	}
}

func TestClaudeMCPConfigIsPrivate(t *testing.T) {
	m := RaenilMCP("https://raenil.example", "raenil_secret", "ag-1", "dev", "ask_user", "propose_tickets")
	path, cleanup, err := claudeMCPConfig(*m)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	b, _ := os.ReadFile(path)
	cleanup()
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("config is %v, want 0600", fi.Mode().Perm())
	}
	for _, want := range []string{`"X-Raenil-Tools":"agent"`, `"Authorization":"Bearer raenil_secret"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("config lacks %s: %s", want, b)
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("cleanup left the config with the token on disk")
	}
	if got := claudeToolNames(*m); got[0] != "mcp__raenil__ask_user" || got[1] != "mcp__raenil__propose_tickets" {
		t.Errorf("claude tool names = %v", got)
	}
}
