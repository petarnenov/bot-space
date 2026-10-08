package integration

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMCPExecutableRequesterAndResponderProcesses(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set TEST_DATABASE_URL for executable MCP integration")
	}
	binary := filepath.Join(t.TempDir(), "requestreply")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, "../../examples/requestreply")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("example build failed: %v %s", err, output)
	}
	f := mailSetup(t)
	srv := mcpHTTP(t, f)
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()
	responder := exec.CommandContext(ctx, binary, "-role", "responder", "-demo-id", "process-test", "-timeout", "15s")
	responder.Env = append(os.Environ(), "MCP_URL="+srv.URL+"/mcp", "MCP_AGENT_TOKEN="+f.tb, "PEER_AGENT_ID="+f.a.ID)
	var responderOutput bytes.Buffer
	responder.Stdout = &responderOutput
	responder.Stderr = &responderOutput
	if err := responder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if responder.Process != nil {
			_ = responder.Process.Kill()
		}
	})
	requester := exec.CommandContext(ctx, binary, "-role", "requester", "-demo-id", "process-test", "-timeout", "15s")
	requester.Env = append(os.Environ(), "MCP_URL="+srv.URL+"/mcp", "MCP_AGENT_TOKEN="+f.ta, "PEER_AGENT_ID="+f.b.ID)
	requesterOutput, err := requester.CombinedOutput()
	if err != nil {
		t.Fatalf("requester failed: %v %s", err, requesterOutput)
	}
	if err = responder.Wait(); err != nil {
		t.Fatalf("responder failed: %v %s", err, responderOutput.String())
	}
	output := string(requesterOutput) + responderOutput.String()
	if !strings.Contains(output, "Requester read and acknowledged reply") || !strings.Contains(output, "Responder read, acknowledged, and replied") {
		t.Fatal("two-process exchange incomplete")
	}
	for _, secret := range []string{f.ta, f.tb, "Synthetic request/reply demonstration", "Synthetic response"} {
		if strings.Contains(output, secret) {
			t.Fatal("example logged tokens or message bodies")
		}
	}
}
