package integration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petarnenov/bot-space/internal/mcpserver"
	"github.com/petarnenov/bot-space/internal/ratelimit"
)

func TestMCPRateSharedCredentialsRefillAndNoRejectedWrite(t *testing.T) {
	f := mailSetup(t)
	_, secondToken, err := f.agents.Issue(f.ctx, f.workspace.ID, f.owner.ID, f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	var seconds atomic.Int64
	seconds.Store(1000)
	clock := func() time.Time { return time.Unix(seconds.Load(), 0) }
	srv := httptest.NewServer(mcpserver.New(f.store, nil, ratelimit.New(60, 2, 100, clock)))
	defer srv.Close()
	first, second := &http.Client{}, &http.Client{}
	request := func(client *http.Client, token, method string, body []byte) *http.Response {
		t.Helper()
		r, _ := http.NewRequest(method, srv.URL, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", "2025-11-25")
		r.Header.Set("Mcp-Name", "send_message")
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response
	}
	// GET is authenticated and admitted before the stateless SDK declines SSE.
	for i, token := range []string{f.ta, secondToken} {
		client := first
		if i == 1 {
			client = second
		}
		if r := request(client, token, "GET", nil); r.StatusCode == 429 || r.StatusCode == 401 {
			t.Fatal("initial authenticated burst rejected")
		}
	}
	body := []byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"send_message\",\"arguments\":{\"to_agent_id\":\"" + f.b.ID + "\",\"idempotency_key\":\"limited-write\",\"text\":\"synthetic\"}}}")
	for _, token := range []string{f.ta, secondToken} {
		r := request(second, token, "POST", body)
		retry, err := strconv.Atoi(r.Header.Get("Retry-After"))
		if r.StatusCode != 429 || err != nil || retry < 1 || retry > 60 {
			t.Fatal("credentials for one identity bypass aggregate quota")
		}
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM mailbox.messages").Scan(&count); err != nil || count != 0 {
		t.Fatal("rate-rejected request persisted a message")
	}
	if r := request(second, f.tb, "GET", nil); r.StatusCode == 429 || r.StatusCode == 401 {
		t.Fatal("unrelated agent shared another agent's quota")
	}
	seconds.Add(1)
	if r := request(first, f.ta, "GET", nil); r.StatusCode == 429 || r.StatusCode == 401 {
		t.Fatal("HTTP admission did not refill")
	}
	if err := f.agents.Revoke(f.ctx, f.workspace.ID, f.owner.ID, f.a.ID, f.ca.ID); err != nil {
		t.Fatal(err)
	}
	if r := request(first, f.ta, "GET", nil); r.StatusCode != 401 {
		t.Fatal("quota obscured revoked credential authentication")
	}
}
