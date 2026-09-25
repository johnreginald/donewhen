package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
)

// MCPServer is Raenil's MCP endpoint as one run should see it: reached as the
// host, naming the agent so what it writes is attributed to it, narrowed to
// the agent tool set and to the ticket's workspace. Each runner hands it to
// its CLI in that CLI's own way.
type MCPServer struct {
	Name      string   // how the CLI names the server, e.g. "raenil"
	URL       string   // the /mcp endpoint
	Token     string   // bearer token; kept out of command lines
	AgentID   string   // X-Raenil-Agent
	Workspace string   // X-Raenil-Workspace
	Tools     []string // bare tool names the run may call without asking
}

// headers are what every request to the server carries, the token excepted.
func (m MCPServer) headers() map[string]string {
	h := map[string]string{"X-Raenil-Tools": "agent", "X-Raenil-Agent": m.AgentID}
	if m.Workspace != "" {
		h["X-Raenil-Workspace"] = m.Workspace
	}
	return h
}

// RaenilMCP describes Raenil's MCP for an agent's run.
func RaenilMCP(baseURL, token, agentID, workspace string, tools ...string) *MCPServer {
	return &MCPServer{Name: "raenil", URL: baseURL + "/mcp", Token: token, AgentID: agentID, Workspace: workspace, Tools: tools}
}

// Tool sets: what a conversation turn and a work run may call without asking.
var (
	ChatMCPTools = []string{"ask_user", "propose_tickets", "get_issue", "list_comments", "get_criteria",
		"list_issues", "get_document", "list_documents"}
	WorkMCPTools = []string{"ask_user"}
)

// claudeMCPConfig writes a private one-run config file for Claude.
func claudeMCPConfig(m MCPServer) (string, func(), error) {
	h := m.headers()
	h["Authorization"] = "Bearer " + m.Token
	b, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{m.Name: map[string]any{
		"type": "http", "url": m.URL, "headers": h,
	}}})
	f, err := os.CreateTemp("", "raenil-mcp-*.json")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	cleanup := func() { os.Remove(name) }
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		cleanup()
		return "", nil, err
	}
	_, werr := f.Write(b)
	f.Close()
	if werr != nil {
		cleanup()
		return "", nil, werr
	}
	return name, cleanup, nil
}

// claudeToolNames are the MCP tools as Claude's permission rules name them.
func claudeToolNames(m MCPServer) []string {
	out := make([]string, len(m.Tools))
	for i, t := range m.Tools {
		out[i] = "mcp__" + m.Name + "__" + t
	}
	return out
}

// codexMCPArgs are the -c overrides that give one codex run the server. The
// token travels in an environment variable, so it is not on the command line.
func codexMCPArgs(m MCPServer, tokenEnv string) []string {
	var hs string
	for k, v := range m.headers() {
		if hs != "" {
			hs += ","
		}
		hs += fmt.Sprintf("%q=%q", k, v)
	}
	p := "mcp_servers." + m.Name + "."
	args := []string{
		"-c", p + "url=" + fmt.Sprintf("%q", m.URL),
		"-c", p + "bearer_token_env_var=" + fmt.Sprintf("%q", tokenEnv),
		"-c", p + "http_headers={" + hs + "}",
	}
	// A non-interactive codex run refuses any MCP call that needs approval,
	// so exactly the run's own tools are approved in advance — nothing else.
	for _, t := range m.Tools {
		args = append(args, "-c", p+"tools."+t+`.approval_mode="approve"`)
	}
	return args
}
