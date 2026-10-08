package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/httpserver"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpclient"
	"github.com/petarnenov/bot-space/internal/mcpserver"
)

func mcpHTTP(t *testing.T, f mailFixture) *httptest.Server {
	t.Helper()
	server := httpserver.New(func(ctx context.Context) error { return database.Ready(ctx, f.pool, bundle(t)) }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewUnstartedServer(server.HTTP.Handler)
	server.Handle("/mcp", mcpserver.New(f.store, []string{"http://" + srv.Listener.Addr().String()}))
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func native(t *testing.T, f mailFixture, srv *httptest.Server, token string) *mcpclient.Client {
	t.Helper()
	client, err := mcpclient.Connect(f.ctx, srv.URL+"/mcp", token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestMCPHTTPDifferentMembersSendReadAcknowledgeReplyRead(t *testing.T) {
	f := mailSetup(t)
	srv := mcpHTTP(t, f)
	a := native(t, f, srv, f.ta)
	b := native(t, f, srv, f.tb)
	var whoA, whoB agents.Principal
	if err := a.Call(f.ctx, "whoami", map[string]any{}, &whoA); err != nil {
		t.Fatal(err)
	}
	if err := b.Call(f.ctx, "whoami", map[string]any{}, &whoB); err != nil {
		t.Fatal(err)
	}
	if whoA.AgentID == whoB.AgentID || whoA.OwnerUserID == whoB.OwnerUserID || whoA.WorkspaceID != whoB.WorkspaceID {
		t.Fatal("not distinct user-owned client identities")
	}
	tools, err := a.Session.ListTools(f.ctx, nil)
	if err != nil || len(tools.Tools) != 5 {
		t.Fatal("five tools not discoverable")
	}
	for _, tool := range tools.Tools {
		encoded, _ := json.Marshal(tool.InputSchema)
		var schema map[string]any
		if json.Unmarshal(encoded, &schema) != nil || schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Fatal("missing strict input schema")
		}
	}
	var directory mailbox.DirectoryPage
	if err := a.Call(f.ctx, "list_agents", map[string]any{}, &directory); err != nil || len(directory.Agents) != 2 {
		t.Fatal("recipient directory failed")
	}
	var sent mailbox.Message
	if err := a.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "http-request", "text": "synthetic request"}, &sent); err != nil {
		t.Fatal(err)
	}
	var inbox mailbox.Page
	if err := b.Call(f.ctx, "read_messages", map[string]any{}, &inbox); err != nil || len(inbox.Messages) != 1 || inbox.Messages[0].ID != sent.ID {
		t.Fatal("HTTP inbox delivery failed")
	}
	var ack mailbox.Message
	if err := b.Call(f.ctx, "acknowledge_message", map[string]any{"message_id": sent.ID}, &ack); err != nil || ack.AcknowledgedAt == nil {
		t.Fatal("HTTP acknowledgement failed")
	}
	var reply mailbox.Message
	if err := b.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.a.ID, "idempotency_key": "http-reply", "text": "synthetic reply", "in_reply_to": sent.ID}, &reply); err != nil {
		t.Fatal(err)
	}
	if err := a.Call(f.ctx, "read_messages", map[string]any{}, &inbox); err != nil || len(inbox.Messages) != 1 || inbox.Messages[0].ID != reply.ID || reply.ThreadID != sent.ThreadID {
		t.Fatal("HTTP reply delivery failed")
	}
	var ignored any
	if err := a.Call(f.ctx, "read_messages", map[string]any{"to_agent_id": f.b.ID}, &ignored); err == nil {
		t.Fatal("foreign inbox argument accepted")
	} else {
		var app *mcpclient.ApplicationError
		if !errors.As(err, &app) || app.Code != "invalid_argument" {
			t.Fatal("unstable validation error")
		}
	}
	if err := a.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "spoof", "text": "deny", "from_agent_id": f.b.ID, "workspace_id": f.workspace.ID}, &ignored); err == nil {
		t.Fatal("spoofed sender accepted")
	}
}

func TestMCPHTTPCredentialOriginAndBodyBoundary(t *testing.T) {
	f := mailSetup(t)
	srv := mcpHTTP(t, f)
	_, browserCookie, err := (&identity.Sessions{Pool: f.pool}).Login(f.ctx, f.owner.GitHubID, "browser-user", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, origin, authorization string
		code                          int
	}{{"POST", "", "", 401}, {"GET", "null", "Bearer " + f.ta, 403}, {"DELETE", "https://foreign.example", "Bearer " + f.ta, 403}, {"POST", "bad-origin", "Bearer " + f.ta, 403}} {
		r, _ := http.NewRequest(tc.method, srv.URL+"/mcp", bytes.NewReader([]byte(`{}`)))
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if tc.authorization != "" {
			r.Header.Set("Authorization", tc.authorization)
		}
		r.AddCookie(&http.Cookie{Name: "bot_space_session", Value: browserCookie})
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.code {
			t.Fatalf("%s origin case: %d", tc.method, response.StatusCode)
		}
	}
	r, _ := http.NewRequest("POST", srv.URL+"/mcp", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Authorization", "Bearer "+f.ta)
	r.Header.Add("Origin", srv.URL)
	r.Header.Add("Origin", srv.URL)
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("duplicate Origin accepted")
	}
	r, _ = http.NewRequest("POST", srv.URL+"/mcp", bytes.NewReader(bytes.Repeat([]byte("x"), httpserver.MaxBodyBytes+1)))
	r.ContentLength = -1
	r.Header.Set("Authorization", "Bearer "+f.ta)
	response, err = http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 413 {
		t.Fatal("chunked oversized body accepted")
	}
	if err := f.agents.Revoke(f.ctx, f.workspace.ID, f.owner.ID, f.a.ID, f.ca.ID); err != nil {
		t.Fatal(err)
	}
	r, _ = http.NewRequest("POST", srv.URL+"/mcp", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("Authorization", "Bearer "+f.ta)
	response, err = http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("revoked protocol credential accepted")
	}
}
