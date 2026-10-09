package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFakeProvider(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-provider.sh")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunProviderRejectsMalformedOutput(t *testing.T) {
	exe := writeFakeProvider(t, `echo 'not-json'`)
	req := ProviderRequest{
		Agent:       AgentConfig{Provider: "codex", Executable: exe, Policy: "analysis", Project: t.TempDir()},
		JobID:       "11111111-1111-4111-8111-111111111111",
		StateDir:    filepath.Join(t.TempDir(), "state"),
		Instruction: "synthetic",
		OnSession:   func(string) error { return nil },
	}
	out, err := RunProvider(context.Background(), req)
	if err == nil || out.ErrorCode != "invalid_provider_output" {
		t.Fatalf("expected invalid provider output, got %#v err=%v", out, err)
	}
}

func TestRunProviderEnforcesResultSizeAndPermissionSignal(t *testing.T) {
	large := strings.Repeat("A", 70000)
	exe := writeFakeProvider(t, "cat <<'EOF'\n"+`{"type":"item.completed","session_id":"11111111-1111-4111-8111-111111111111","item":{"type":"agent_message","text":"`+large+`"}}`+"\nEOF")
	req := ProviderRequest{
		Agent:       AgentConfig{Provider: "codex", Executable: exe, Policy: "analysis", Project: t.TempDir()},
		JobID:       "22222222-2222-4222-8222-222222222222",
		StateDir:    filepath.Join(t.TempDir(), "state"),
		Instruction: "synthetic",
		OnSession:   func(string) error { return nil },
	}
	out, err := RunProvider(context.Background(), req)
	if err == nil || out.ErrorCode != "output_overflow" {
		t.Fatalf("expected output overflow, got %#v err=%v", out, err)
	}

	claude := writeFakeProvider(t, `echo '{"type":"result","subtype":"success","result":"ok","permission_denials":[{}]}'`)
	req.Agent.Provider = "claude"
	req.Agent.Executable = claude
	req.JobID = "33333333-3333-4333-8333-333333333333"
	out, err = RunProvider(context.Background(), req)
	if err != nil || out.ErrorCode != "requires_approval" || out.Text != "ok" {
		t.Fatalf("expected permission escalation signal, got %#v err=%v", out, err)
	}
}

func TestProviderCommandUsesPrivateInputFile(t *testing.T) {
	exe := writeFakeProvider(t, `echo '{}'`)
	state := filepath.Join(t.TempDir(), "state")
	req := ProviderRequest{
		Agent:       AgentConfig{Provider: "copilot", Executable: exe, Policy: "analysis", Project: t.TempDir()},
		JobID:       "44444444-4444-4444-8444-444444444444",
		StateDir:    state,
		Instruction: "private-sentinel",
		OnSession:   func(string) error { return nil },
	}
	cmd, err := providerCommand(req)
	if err != nil {
		t.Fatal(err)
	}
	inputPath := ""
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "BOT_SPACE_TASK_INPUT=") {
			inputPath = strings.TrimPrefix(env, "BOT_SPACE_TASK_INPUT=")
			break
		}
	}
	if inputPath == "" || !strings.HasPrefix(inputPath, filepath.Join(state, "provider", req.JobID)) {
		t.Fatal("provider input path is not private and fixed under state dir")
	}
	raw, err := os.ReadFile(inputPath)
	if err != nil || string(raw) != "private-sentinel" {
		t.Fatal("instruction was not persisted in private input file")
	}
	info, err := os.Stat(inputPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private input file permissions are not owner-only")
	}
}

func TestRunProviderPreservesExactSessionIdentity(t *testing.T) {
	session := "55555555-5555-4555-8555-555555555555"
	event := map[string]any{"type": "assistant.message", "session_id": session, "data": map[string]any{"content": "done"}}
	raw, _ := json.Marshal(event)
	exe := writeFakeProvider(t, "cat <<'EOF'\n"+string(raw)+"\nEOF")
	var seen string
	req := ProviderRequest{
		Agent:       AgentConfig{Provider: "copilot", Executable: exe, Policy: "analysis", Project: t.TempDir()},
		JobID:       "66666666-6666-4666-8666-666666666666",
		StateDir:    filepath.Join(t.TempDir(), "state"),
		SessionID:   session,
		Instruction: "synthetic",
		OnSession:   func(s string) error { seen = s; return nil },
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	out, err := RunProvider(ctx, req)
	if err != nil || out.SessionID != session || seen != session || out.Text != "done" {
		t.Fatalf("expected exact session continuity, got %#v seen=%q err=%v", out, seen, err)
	}
}
