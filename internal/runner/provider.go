package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/tasks"
)

type ProviderResult struct{ Text, SessionID, ErrorCode string }
type ProviderRequest struct {
	Agent                                               AgentConfig
	JobID, StateDir, Capability, SessionID, Instruction string
	OnSession                                           func(string) error
}

const maxProviderBytes = 8 << 20

func providerEnvironment(provider, state, capability, input string) []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "USER": true, "SHELL": true, "TMPDIR": true, "LANG": true, "LC_ALL": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true}
	switch provider {
	case "codex":
		allowed["CODEX_API_KEY"] = true
		allowed["OPENAI_API_KEY"] = true
	case "claude":
		allowed["ANTHROPIC_API_KEY"] = true
		allowed["CLAUDE_CODE_OAUTH_TOKEN"] = true
	case "copilot":
		allowed["COPILOT_GITHUB_TOKEN"] = true
		allowed["GH_TOKEN"] = true
		allowed["GITHUB_TOKEN"] = true
	}
	env := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if allowed[key] {
			env = append(env, entry)
		}
	}
	return append(env, "BOT_SPACE_JOB_CAP="+capability, "BOT_SPACE_TASK_INPUT="+input, "BOT_SPACE_STATE_DIR="+state, "DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1")
}

func providerCommand(request ProviderRequest) (*exec.Cmd, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, errors.New("runner executable unavailable")
	}
	jobDir := filepath.Join(request.StateDir, "provider", request.JobID)
	if err = os.MkdirAll(jobDir, 0700); err != nil {
		return nil, errors.New("private provider directory unavailable")
	}
	inputPath := filepath.Join(jobDir, "input.txt")
	if err = os.WriteFile(inputPath, []byte(request.Instruction), 0600); err != nil {
		return nil, errors.New("private provider input unavailable")
	}
	bridgeArgs := []string{"bridge", "--state", request.StateDir, "--job-id", request.JobID}
	bridge := map[string]any{"command": binary, "args": bridgeArgs, "env": map[string]string{"BOT_SPACE_JOB_CAP": "$" + "{BOT_SPACE_JOB_CAP}"}, "timeout": 7200000, "tools": []string{"delegate_task", "whoami", "list_agents"}}
	configPath := filepath.Join(jobDir, "mcp.json")
	server := map[string]any{"mcpServers": map[string]any{"bot_space": bridge}}
	if request.Agent.Provider == "copilot" {
		bridge["type"] = "local"
	}
	if err = atomicJSON(configPath, server); err != nil {
		return nil, err
	}
	var args []string
	switch request.Agent.Provider {
	case "codex":
		mode := "read-only"
		if request.Agent.Policy == "workspace-write" {
			mode = "workspace-write"
		}
		commandJSON, _ := json.Marshal(binary)
		argsJSON, _ := json.Marshal(bridgeArgs)
		args = []string{"-c", "approval_policy=\"never\"", "-c", "sandbox_mode=" + fmt.Sprintf("%q", mode), "-c", "mcp_servers={}", "-c", "mcp_servers.bot_space.command=" + string(commandJSON), "-c", "mcp_servers.bot_space.args=" + string(argsJSON), "-c", "mcp_servers.bot_space.env_vars=[\"BOT_SPACE_JOB_CAP\"]", "-c", "mcp_servers.bot_space.tool_timeout_sec=7200"}
		if request.Agent.Model != "" {
			args = append(args, "-m", request.Agent.Model)
		}
		if request.Agent.Effort != "" {
			args = append(args, "-c", "model_reasoning_effort="+fmt.Sprintf("%q", request.Agent.Effort))
		}
		args = append(args, "exec")
		if request.SessionID != "" {
			args = append(args, "resume", "--json", request.SessionID, "-")
		} else {
			args = append(args, "--json", "--color", "never", "-")
		}
	case "claude":
		tools := "Read,Glob,Grep"
		if request.Agent.Policy == "workspace-write" {
			tools += ",Edit,Write"
		}
		args = []string{"-p", "--output-format", "stream-json", "--verbose", "--permission-mode", "dontAsk", "--strict-mcp-config", "--mcp-config", configPath, "--tools", tools, "--allowedTools", tools + ",mcp__bot_space__delegate_task,mcp__bot_space__whoami,mcp__bot_space__list_agents"}
		if request.SessionID != "" {
			args = append(args, "--resume", request.SessionID)
		}
		if request.Agent.Model != "" {
			args = append(args, "--model", request.Agent.Model)
		}
		if request.Agent.Effort != "" {
			args = append(args, "--effort", request.Agent.Effort)
		}
	case "copilot":
		sid := request.SessionID
		if sid == "" {
			sid, err = randomID()
			if err != nil {
				return nil, err
			}
			if err = request.OnSession(sid); err != nil {
				return nil, err
			}
		}
		tools := "view,grep,glob,bot_space"
		if request.Agent.Policy == "workspace-write" {
			tools += ",edit,create"
		}
		args = []string{"-p", "Read the assigned instruction from the file named by BOT_SPACE_TASK_INPUT. Follow local execution permissions. Delegate via bot_space when requested; use a stable request_key for each delegation.", "--session-id", sid, "--output-format", "json", "--log-level", "none", "--no-ask-user", "--no-auto-update", "--disable-builtin-mcps", "--additional-mcp-config", "@" + configPath, "--add-dir", jobDir, "--available-tools", tools, "--allow-tool", tools, "--deny-tool", "shell", "--secret-env-vars", "COPILOT_GITHUB_TOKEN,GH_TOKEN,GITHUB_TOKEN"}
		if request.Agent.Model != "" {
			args = append(args, "--model", request.Agent.Model)
		}
		if request.Agent.Effort != "" {
			args = append(args, "--reasoning-effort", request.Agent.Effort)
		}
	default:
		return nil, errors.New("unsupported provider")
	}
	command := exec.Command(request.Agent.Executable, args...)
	command.Dir = request.Agent.Project
	command.Env = providerEnvironment(request.Agent.Provider, request.StateDir, request.Capability, inputPath)
	if request.Agent.Provider != "copilot" {
		command.Stdin = strings.NewReader(request.Instruction)
	}
	command.Stderr = io.Discard
	return command, nil
}

type providerEvent struct {
	Type              string            `json:"type"`
	SessionID         string            `json:"session_id"`
	ThreadID          string            `json:"thread_id"`
	Subtype           string            `json:"subtype"`
	Result            string            `json:"result"`
	IsError           bool              `json:"is_error"`
	PermissionDenials []json.RawMessage `json:"permission_denials"`
	Item              struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
	Data struct {
		Content   string `json:"content"`
		Text      string `json:"text"`
		SessionID string `json:"sessionId"`
	} `json:"data"`
}

func RunProvider(ctx context.Context, request ProviderRequest) (ProviderResult, error) {
	if !security.ValidUUID(request.JobID) || request.OnSession == nil {
		return ProviderResult{}, errors.New("invalid provider job")
	}
	sessionID := request.SessionID
	recordSession := request.OnSession
	request.OnSession = func(id string) error {
		if sessionID != "" && id != sessionID {
			return errors.New("provider resumed an unexpected session")
		}
		if err := recordSession(id); err != nil {
			return err
		}
		sessionID = id
		return nil
	}
	cmd, err := providerCommand(request)
	if err != nil {
		return ProviderResult{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ProviderResult{}, errors.New("provider output unavailable")
	}
	stop, err := startProcess(ctx, cmd)
	if err != nil {
		return ProviderResult{}, errors.New("provider process could not start")
	}
	defer stop()
	result := ProviderResult{SessionID: sessionID}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 8192), 1<<20)
	total := 0
	for scanner.Scan() {
		total += len(scanner.Bytes())
		if total > maxProviderBytes {
			stop()
			_ = cmd.Wait()
			return ProviderResult{ErrorCode: "output_overflow"}, errors.New("provider output limit exceeded")
		}
		var event providerEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			stop()
			_ = cmd.Wait()
			return ProviderResult{ErrorCode: "invalid_provider_output"}, errors.New("provider emitted invalid structured output")
		}
		sid := event.SessionID
		if event.ThreadID != "" {
			sid = event.ThreadID
		}
		if event.Data.SessionID != "" {
			sid = event.Data.SessionID
		}
		if sid != "" {
			if !security.ValidUUID(sid) || result.SessionID != "" && sid != result.SessionID {
				stop()
				_ = cmd.Wait()
				return ProviderResult{ErrorCode: "session_mismatch"}, errors.New("provider resumed an unexpected session")
			}
			if err = request.OnSession(sid); err != nil {
				stop()
				_ = cmd.Wait()
				return ProviderResult{}, err
			}
			result.SessionID = sid
		}
		switch request.Agent.Provider {
		case "codex":
			if event.Type == "item.completed" && event.Item.Type == "agent_message" {
				result.Text = event.Item.Text
			}
			if event.Type == "error" || event.Type == "turn.failed" {
				result.ErrorCode = "provider_failed"
			}
		case "claude":
			if event.Type == "result" {
				result.Text = event.Result
				if event.IsError || event.Subtype != "success" {
					result.ErrorCode = "provider_failed"
				}
				if len(event.PermissionDenials) > 0 {
					result.ErrorCode = "requires_approval"
				}
			}
		case "copilot":
			if event.Type == "assistant.message" {
				result.Text = event.Data.Content
				if result.Text == "" {
					result.Text = event.Data.Text
				}
			}
			if event.Type == "session.error" {
				result.ErrorCode = "provider_failed"
			}
		}
		if len(result.Text) > tasks.MaxResultBytes {
			stop()
			_ = cmd.Wait()
			return ProviderResult{ErrorCode: "output_overflow"}, errors.New("provider result limit exceeded")
		}
	}
	if scanner.Err() != nil {
		stop()
		_ = cmd.Wait()
		return ProviderResult{ErrorCode: "output_overflow"}, errors.New("provider stream limit or read failure")
	}
	if err = cmd.Wait(); err != nil {
		return ProviderResult{SessionID: result.SessionID, ErrorCode: "provider_failed"}, errors.New("provider execution failed")
	}
	if ctx.Err() != nil {
		return ProviderResult{SessionID: result.SessionID, ErrorCode: "interrupted"}, ctx.Err()
	}
	if result.SessionID == "" && request.Agent.Provider == "copilot" {
		result.SessionID = request.SessionID
	}
	if result.Text == "" && result.ErrorCode == "" {
		return ProviderResult{ErrorCode: "invalid_provider_output"}, errors.New("provider returned no final result")
	}
	return result, nil
}
