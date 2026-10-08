package integration

import (
	"sync"
	"testing"

	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpclient"
)

func TestMCPHTTPConcurrentIdempotencyAndDirectoryEligibility(t *testing.T) {
	f := mailSetup(t)
	srv := mcpHTTP(t, f)
	one := native(t, f, srv, f.ta)
	two := native(t, f, srv, f.ta)
	var wg sync.WaitGroup
	type outcome struct {
		m   mailbox.Message
		err error
	}
	results := make(chan outcome, 2)
	for _, client := range []*mcpclient.Client{one, two} {
		wg.Add(1)
		go func(client *mcpclient.Client) {
			defer wg.Done()
			var m mailbox.Message
			err := client.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "parallel-http", "text": "synthetic"}, &m)
			results <- outcome{m, err}
		}(client)
	}
	wg.Wait()
	close(results)
	id := ""
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if id == "" {
			id = result.m.ID
		}
		if id != result.m.ID {
			t.Fatal("HTTP concurrent key duplicated delivery")
		}
	}
	var ignored any
	if err := one.Call(f.ctx, "send_message", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "parallel-http", "text": "changed"}, &ignored); appCode(err) != "idempotency_conflict" {
		t.Fatal("HTTP conflict code changed")
	}
	other, err := f.teams.Bootstrap(f.ctx, 201, "directory-foreign")
	if err != nil {
		t.Fatal(err)
	}
	outside := user(t, f.ctx, f.pool, 201)
	agent(t, f.ctx, f.agents, other, outside, "outside")
	var directory mailbox.DirectoryPage
	if err := one.Call(f.ctx, "list_agents", map[string]any{}, &directory); err != nil || len(directory.Agents) != 2 {
		t.Fatal("directory crossed workspace")
	}
	for _, a := range directory.Agents {
		if a.WorkspaceID != f.workspace.ID {
			t.Fatal("foreign directory entry")
		}
	}
	if err = f.agents.Deactivate(f.ctx, f.workspace.ID, f.owner.ID, f.b.ID); err != nil {
		t.Fatal(err)
	}
	if err := one.Call(f.ctx, "list_agents", map[string]any{}, &directory); err != nil || len(directory.Agents) != 1 || directory.Agents[0].ID != f.a.ID {
		t.Fatal("inactive directory entry remained")
	}
}
