package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpclient"
)

func TestRealApplicationProcessRestartPreservesOfflineMailbox(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set TEST_DATABASE_URL for process restart integration")
	}
	binary := filepath.Join(t.TempDir(), "mailbox")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, "../../cmd/mailbox")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("application build failed: %v %s", err, output)
	}
	f := mailSetup(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	env := append(os.Environ(), "DATABASE_URL="+f.pool.Config().ConnConfig.ConnString(), "PORT="+strconv.Itoa(port), "CURSOR_SIGNING_KEY="+hex.EncodeToString(bytes.Repeat([]byte{3}, 32)), "GITHUB_CLIENT_ID=", "GITHUB_CLIENT_SECRET=", "PUBLIC_BASE_URL=", "MCP_ALLOWED_ORIGINS=")
	start := func() (*exec.Cmd, *bytes.Buffer) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, binary, "serve")
		cmd.Env = env
		var logs bytes.Buffer
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			response, err := http.Get(base + "/readyz")
			if err == nil {
				response.Body.Close()
				if response.StatusCode == 200 {
					return cmd, &logs
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("application process readiness failed")
		return nil, nil
	}
	first, firstLogs := start()
	client, err := mcpclient.Connect(f.ctx, base+"/mcp", f.ta)
	if err != nil {
		t.Fatal(err)
	}
	var sent mailbox.Message
	if err = client.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "process-offline", "text": "private-process-body-sentinel"}, &sent); err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	if err = first.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err = first.Wait(); err != nil {
		t.Fatal("first process shutdown failed")
	}
	second, secondLogs := start()
	if second.Process.Pid == first.Process.Pid {
		t.Fatal("application process not replaced")
	}
	recipient, err := mcpclient.Connect(f.ctx, base+"/mcp", f.tb)
	if err != nil {
		t.Fatal(err)
	}
	var page mailbox.Page
	if err = recipient.Call(f.ctx, "read_messages", map[string]any{}, &page); err != nil || len(page.Messages) != 1 || page.Messages[0].ID != sent.ID || page.Messages[0].Text != sent.Text {
		t.Fatal("offline message lost through actual process restart")
	}
	_ = recipient.Close()
	if err = second.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err = second.Wait(); err != nil {
		t.Fatal("second process shutdown failed")
	}
	logs := firstLogs.String() + secondLogs.String()
	for _, secret := range []string{f.ta, f.tb, sent.Text, f.pool.Config().ConnConfig.ConnString()} {
		if strings.Contains(logs, secret) {
			t.Fatal("application logged credential, database URL or message body")
		}
	}
}
