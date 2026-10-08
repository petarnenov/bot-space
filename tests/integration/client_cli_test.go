package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledClientCLIConfiguration(t *testing.T) {
	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("Codex CLI is not installed; native SDK acceptance remains mandatory")
	}
	cmd := exec.Command(codex, "-c", "mcp_servers.bot_space.url=\"http://127.0.0.1:8080/mcp\"", "-c", "mcp_servers.bot_space.bearer_token_env_var=\"MCP_AGENT_TOKEN\"", "mcp", "get", "bot_space", "--json")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("Codex CLI configuration parsing failed")
	}
	var result struct {
		Transport struct {
			Type   string
			Bearer string `json:"bearer_token_env_var"`
		}
	}
	if json.Unmarshal(output, &result) != nil || result.Transport.Type != "streamable_http" || result.Transport.Bearer != "MCP_AGENT_TOKEN" {
		t.Fatal("Codex bearer configuration was not recognized")
	}
}

func TestInstalledClaudeCLIConnectsWithEnvironmentBearer(t *testing.T) {
	claude, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("Claude Code CLI is not installed; native SDK acceptance remains mandatory")
	}
	f := mailSetup(t)
	srv := mcpHTTP(t, f)
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.Mkdir(config, 0700); err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{"type": "http", "url": srv.URL + "/mcp", "headers": map[string]string{"Authorization": "Bearer $" + "{MCP_AGENT_TOKEN}"}}
	encoded, _ := json.Marshal(entry)
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(f.ctx, claude, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+config, "MCP_AGENT_TOKEN="+f.ta, "DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal("isolated Claude MCP configuration/check failed")
		}
		if strings.Contains(string(output), f.ta) {
			t.Fatal("Claude CLI exposed raw credential")
		}
		return output
	}
	run("mcp", "add-json", "--scope", "user", "bot_space", string(encoded))
	output := run("mcp", "get", "bot_space")
	if !strings.Contains(string(output), "Connected") {
		t.Fatal("Claude CLI did not establish an authenticated MCP connection")
	}
}
