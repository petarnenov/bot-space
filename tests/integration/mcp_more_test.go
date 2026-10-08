package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpclient"
)

func appCode(err error) string {
	var app *mcpclient.ApplicationError
	if errors.As(err, &app) {
		return app.Code
	}
	return ""
}

func TestMCPHTTPIsolationErrorsPrecisionAndResultLimits(t *testing.T) {
	f := mailSetup(t)
	srv := mcpHTTP(t, f)
	a := native(t, f, srv, f.ta)
	b := native(t, f, srv, f.tb)
	other, err := f.teams.Bootstrap(f.ctx, 201, "foreign-mcp")
	if err != nil {
		t.Fatal(err)
	}
	outsider := user(t, f.ctx, f.pool, 201)
	d, _, td := agent(t, f.ctx, f.agents, other, outsider, "foreign")
	foreignClient := native(t, f, srv, td)
	var result any
	if err := a.Call(f.ctx, "send_message", map[string]any{"to_agent_id": d.ID, "idempotency_key": "foreign", "text": "deny"}, &result); appCode(err) != "not_found" {
		t.Fatal("cross-workspace HTTP send allowed")
	}
	for _, arguments := range []map[string]any{
		{"to_agent_id": f.b.ID, "idempotency_key": "empty", "text": ""},
		{"to_agent_id": f.b.ID, "idempotency_key": "large", "text": strings.Repeat("x", mailbox.MaxTextBytes+1)},
		{"to_agent_id": "invalid", "idempotency_key": "invalid-uuid", "text": "valid"},
		{"to_agent_id": f.b.ID, "idempotency_key": "metadata", "text": "valid", "metadata": map[string]any{"x": strings.Repeat("x", mailbox.MaxMetadataBytes)}},
		{"to_agent_id": f.b.ID, "idempotency_key": "null", "text": "valid", "kind": nil},
	} {
		if err := a.Call(f.ctx, "send_message", arguments, &result); appCode(err) != "invalid_argument" {
			t.Fatal("unstable payload validation")
		}
	}
	var value any = "deep"
	for i := 0; i < 9; i++ {
		value = map[string]any{"nested": value}
	}
	if err := a.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "depth", "text": "valid", "metadata": value}, &result); appCode(err) != "invalid_argument" {
		t.Fatal("metadata depth accepted")
	}
	var sent mailbox.Message
	raw := json.RawMessage(`{"to_agent_id":"` + f.b.ID + `","idempotency_key":"exact","text":"number fidelity","metadata":{"large":9007199254740993,"exponent":1e100}}`)
	if err := a.Call(f.ctx, "send_message", raw, &sent); err != nil || !strings.Contains(string(sent.Metadata), "9007199254740993") || !strings.Contains(string(sent.Metadata), "1e100") {
		t.Fatalf("metadata precision/compact exponent lost: %v", err)
	}
	if err := a.Call(f.ctx, "acknowledge_message", map[string]any{"message_id": sent.ID}, &result); appCode(err) != "not_found" {
		t.Fatal("HTTP sender acknowledged foreign inbox")
	}
	var foreignMessage mailbox.Message
	if err := foreignClient.Call(f.ctx, "send_message", map[string]any{"to_agent_id": d.ID, "idempotency_key": "foreign-thread", "text": "foreign"}, &foreignMessage); err != nil {
		t.Fatal(err)
	}
	if err := a.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "foreign-reference", "text": "deny", "in_reply_to": foreignMessage.ID}, &result); appCode(err) != "not_found" {
		t.Fatal("foreign reply linked over HTTP")
	}
	for _, key := range []string{"big-one", "big-two", "big-three"} {
		if err := a.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": key, "text": strings.Repeat("<", mailbox.MaxTextBytes)}, &result); err != nil {
			t.Fatal(err)
		}
	}
	actual, err := b.Session.CallTool(f.ctx, &mcp.CallToolParams{Name: "read_messages", Arguments: map[string]any{}})
	if err != nil || actual.IsError {
		t.Fatal("large inbox failed")
	}
	encoded, _ := json.Marshal(actual)
	if len(encoded) > mailbox.MaxToolResultBytes {
		t.Fatal("actual SDK result exceeded cap")
	}
	var page mailbox.Page
	if err := b.Call(f.ctx, "read_messages", map[string]any{}, &page); err != nil || page.NextCursor == nil {
		t.Fatal("large result has no continuation")
	}
	if err := a.Call(f.ctx, "read_messages", map[string]any{"cursor": *page.NextCursor}, &result); appCode(err) != "invalid_argument" {
		t.Fatal("foreign SDK cursor accepted")
	}
	if err := b.Call(f.ctx, "read_messages", map[string]any{"cursor": *page.NextCursor, "acknowledged": "all"}, &result); appCode(err) != "invalid_argument" {
		t.Fatal("changed-filter SDK cursor accepted")
	}
	if err := b.Call(f.ctx, "read_messages", map[string]any{"limit": 0}, &result); appCode(err) != "invalid_argument" {
		t.Fatal("invalid explicit page limit accepted")
	}
	if err := b.Call(f.ctx, "whoami", map[string]any{"agent_id": f.a.ID}, &result); appCode(err) != "invalid_argument" {
		t.Fatal("whoami identity substitution accepted")
	}
}

func TestMCPHTTPOfflineDeliveryAfterApplicationReconstruction(t *testing.T) {
	f := mailSetup(t)
	first := mcpHTTP(t, f)
	sender := native(t, f, first, f.ta)
	var sent mailbox.Message
	if err := sender.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "offline", "text": "synthetic offline delivery"}, &sent); err != nil {
		t.Fatal(err)
	}
	_ = sender.Close()
	first.Close()
	reconstructed, err := mailbox.New(f.pool, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	f.store = reconstructed
	second := mcpHTTP(t, f)
	recipient := native(t, f, second, f.tb)
	var page mailbox.Page
	if err := recipient.Call(f.ctx, "read_messages", map[string]any{}, &page); err != nil || len(page.Messages) != 1 || page.Messages[0].ID != sent.ID || page.Messages[0].Text != sent.Text {
		t.Fatal("offline message lost after reconstructing application")
	}
	if err := f.teams.Remove(f.ctx, f.workspace.ID, f.owner.ID, f.member.ID); err != nil {
		t.Fatal(err)
	}
	if err := recipient.Call(f.ctx, "whoami", map[string]any{}, &agents.Principal{}); err == nil {
		t.Fatal("removed member retained HTTP MCP access")
	}
}

func TestMCPHTTPPreservesSDKProtocolAndLocalhostProtection(t *testing.T) {
	f := mailSetup(t)
	srv := mcpHTTP(t, f)
	r, _ := http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader("{malformed"))
	r.Header.Set("Authorization", "Bearer "+f.ta)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatalf("SDK malformed protocol status: %d", response.StatusCode)
	}
	r, _ = http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader(`{}`))
	r.Host = "untrusted.example"
	r.Header.Set("Authorization", "Bearer "+f.ta)
	response, err = http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("SDK localhost rebinding protection disabled")
	}
}

func TestSDKExtremeNumberIsExactOrClearlyRejected(t *testing.T) {
	f := mailSetup(t)
	srv := mcpHTTP(t, f)
	a := native(t, f, srv, f.ta)
	arguments := json.RawMessage(`{"to_agent_id":"` + f.b.ID + `","idempotency_key":"sdk-number-limit","text":"synthetic","metadata":{"n":1e1000000}}`)
	result, err := a.Session.CallTool(f.ctx, &mcp.CallToolParams{Name: "send_message", Arguments: arguments})
	if err == nil && !result.IsError {
		if len(result.Content) != 1 {
			t.Fatal("unexpected successful SDK result")
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatal("unexpected SDK content")
		}
		var message mailbox.Message
		if json.Unmarshal([]byte(text.Text), &message) != nil || !strings.Contains(string(message.Metadata), "1e1000000") {
			t.Fatal("accepted SDK number lost precision")
		}
	} else {
		var count int
		if f.pool.QueryRow(f.ctx, "SELECT count(*) FROM mailbox.messages WHERE workspace_id=$1 AND idempotency_key='sdk-number-limit'", f.workspace.ID).Scan(&count) != nil || count != 0 {
			t.Fatal("rejected SDK number was silently stored")
		}
	}
}
