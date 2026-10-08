package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

func pointer[T any](value T) *T { return &value }

type mailFixture struct {
	ctx           context.Context
	pool          *pgxpool.Pool
	teams         *workspaces.Store
	agents        *agents.Store
	store         *mailbox.Store
	workspace     workspaces.Workspace
	owner, member workspaces.User
	a, b          agents.Agent
	ca, cb        agents.Credential
	ta, tb        string
}

func mailSetup(t *testing.T) mailFixture {
	t.Helper()
	ctx, pool, teams, w, owner := teams(t)
	member := join(t, ctx, pool, teams, w, owner, 102, "member")
	agentsStore := &agents.Store{Pool: pool}
	a, ca, ta := agent(t, ctx, agentsStore, w, owner, "requester")
	b, cb, tb := agent(t, ctx, agentsStore, w, member, "responder")
	store, err := mailbox.New(pool, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return mailFixture{ctx, pool, teams, agentsStore, store, w, owner, member, a, b, ca, cb, ta, tb}
}

func TestMailboxSendReadAcknowledgeReply(t *testing.T) {
	f := mailSetup(t)
	request, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "request", Text: "synthetic request", Metadata: map[string]any{"large": json.Number("9007199254740993")}})
	if err != nil {
		t.Fatal(err)
	}
	if request.FromAgentID != f.a.ID || request.ToAgentID != f.b.ID || request.WorkspaceID != f.workspace.ID || request.AcknowledgedAt != nil || !strings.Contains(string(request.Metadata), "9007199254740993") {
		t.Fatal("message identity or metadata corrupted")
	}
	for i := 0; i < 2; i++ {
		page, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{})
		if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != request.ID || page.Messages[0].AcknowledgedAt != nil {
			t.Fatal("read changed processing state")
		}
	}
	if _, err = f.store.Acknowledge(f.ctx, f.ta, request.ID); !errors.Is(err, mailbox.ErrNotFound) {
		t.Fatal("sender acknowledged recipient inbox")
	}
	one, err := f.store.Acknowledge(f.ctx, f.tb, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	two, err := f.store.Acknowledge(f.ctx, f.tb, request.ID)
	if err != nil || one.AcknowledgedAt == nil || two.AcknowledgedAt == nil || !one.AcknowledgedAt.Equal(*two.AcknowledgedAt) {
		t.Fatal("acknowledgement not idempotent")
	}
	page, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{})
	if err != nil || len(page.Messages) != 0 {
		t.Fatal("acknowledged entry remains in default filter")
	}
	page, err = f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Acknowledged: pointer("all")})
	if err != nil || len(page.Messages) != 1 {
		t.Fatal("acknowledged data deleted")
	}
	reply, err := f.store.Send(f.ctx, f.tb, mailbox.SendInput{ToAgentID: f.a.ID, IdempotencyKey: "reply", Text: "synthetic reply", InReplyTo: &request.ID})
	if err != nil || reply.ThreadID != request.ThreadID || reply.InReplyTo == nil || *reply.InReplyTo != request.ID {
		t.Fatalf("reply failed: %v", err)
	}
	inbox, err := f.store.Read(f.ctx, f.ta, mailbox.ReadInput{})
	if err != nil || len(inbox.Messages) != 1 || inbox.Messages[0].ID != reply.ID {
		t.Fatal("reply not delivered")
	}
}

func TestMailboxIdempotencyConcurrencyAndRotation(t *testing.T) {
	f := mailSetup(t)
	input := mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "same-key", Text: "payload", Metadata: map[string]any{"n": json.Number("1.0"), "large": json.Number("9007199254740993")}}
	var wg sync.WaitGroup
	results := make(chan mailbox.Message, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m, err := f.store.Send(f.ctx, f.ta, input); results <- m; failures <- err }()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for m := range results {
		if id == "" {
			id = m.ID
		}
		if m.ID != id {
			t.Fatal("duplicate keyed delivery")
		}
	}
	equivalent := input
	equivalent.Metadata = map[string]any{"large": json.Number("9007199254740993"), "n": json.Number("1e0")}
	equivalent.Kind = pointer("message")
	if m, err := f.store.Send(f.ctx, f.ta, equivalent); err != nil || m.ID != id {
		t.Fatal("equivalent metadata/defaults conflicted")
	}
	conflicting := input
	conflicting.Text = "different"
	if _, err := f.store.Send(f.ctx, f.ta, conflicting); !errors.Is(err, mailbox.ErrConflict) {
		t.Fatal("changed payload did not conflict")
	}
	_, newToken, err := f.agents.Rotate(f.ctx, f.workspace.ID, f.owner.ID, f.a.ID, f.ca.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := f.store.Send(f.ctx, newToken, input); err != nil || m.ID != id {
		t.Fatal("rotation changed idempotency scope")
	}
	if err := f.agents.Deactivate(f.ctx, f.workspace.ID, f.owner.ID, f.b.ID); err != nil {
		t.Fatal(err)
	}
	if m, err := f.store.Send(f.ctx, newToken, input); err != nil || m.ID != id {
		t.Fatal("committed retry depended on current recipient activation")
	}
	fresh := input
	fresh.IdempotencyKey = "fresh"
	if _, err := f.store.Send(f.ctx, newToken, fresh); !errors.Is(err, mailbox.ErrNotFound) {
		t.Fatal("new message sent to inactive agent")
	}
	var count int
	if f.pool.QueryRow(f.ctx, "SELECT count(*) FROM mailbox.messages WHERE workspace_id=$1", f.workspace.ID).Scan(&count) != nil || count != 1 {
		t.Fatal("duplicate persisted messages")
	}
}

func TestMailboxWorkspaceAndReferenceIsolation(t *testing.T) {
	f := mailSetup(t)
	thirdUser := join(t, f.ctx, f.pool, f.teams, f.workspace, f.owner, 103, "member")
	c, _, tc := agent(t, f.ctx, f.agents, f.workspace, thirdUser, "third")
	other, err := f.teams.Bootstrap(f.ctx, 201, "other-workspace")
	if err != nil {
		t.Fatal(err)
	}
	otherUser := user(t, f.ctx, f.pool, 201)
	d, _, td := agent(t, f.ctx, f.agents, other, otherUser, "foreign")
	request, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "visible", Text: "private"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.store.Send(f.ctx, td, mailbox.SendInput{ToAgentID: d.ID, IdempotencyKey: "foreign-self", Text: "foreign data"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []mailbox.SendInput{
		{ToAgentID: d.ID, IdempotencyKey: "foreign-recipient", Text: "deny"},
		{ToAgentID: f.b.ID, IdempotencyKey: "foreign-thread", Text: "deny", ThreadID: &foreign.ThreadID},
		{ToAgentID: f.b.ID, IdempotencyKey: "foreign-reply", Text: "deny", InReplyTo: &foreign.ID},
		{ToAgentID: c.ID, IdempotencyKey: "wrong-pair", Text: "deny", ThreadID: &request.ThreadID},
	} {
		if _, err := f.store.Send(f.ctx, f.ta, input); !errors.Is(err, mailbox.ErrNotFound) {
			t.Fatal("foreign or mismatched link allowed")
		}
	}
	if _, err := f.store.Send(f.ctx, tc, mailbox.SendInput{ToAgentID: f.a.ID, IdempotencyKey: "third-party", Text: "deny", InReplyTo: &request.ID}); !errors.Is(err, mailbox.ErrNotFound) {
		t.Fatal("third party referenced invisible message")
	}
	separate, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "another-thread", Text: "another"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Send(f.ctx, f.tb, mailbox.SendInput{ToAgentID: f.a.ID, IdempotencyKey: "mismatched-reply-thread", Text: "deny", ThreadID: &request.ThreadID, InReplyTo: &separate.ID}); !errors.Is(err, mailbox.ErrNotFound) {
		t.Fatal("inconsistent thread and reply accepted")
	}
	page, err := f.store.Read(f.ctx, tc, mailbox.ReadInput{})
	if err != nil || len(page.Messages) != 0 {
		t.Fatal("third party read another inbox")
	}
	if _, err := f.store.Acknowledge(f.ctx, tc, request.ID); !errors.Is(err, mailbox.ErrNotFound) {
		t.Fatal("third-party acknowledgement")
	}
	if err := f.teams.Remove(f.ctx, f.workspace.ID, f.owner.ID, thirdUser.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: c.ID, IdempotencyKey: "removed", Text: "deny"}); !errors.Is(err, mailbox.ErrNotFound) {
		t.Fatal("removed-owner recipient allowed")
	}
}

func TestMailboxSnapshotPaginationAndByteLimits(t *testing.T) {
	f := mailSetup(t)
	for i, key := range []string{"one", "two", "three"} {
		if _, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: key, Text: strings.Repeat("<", mailbox.MaxTextBytes), Metadata: map[string]any{"index": i}}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Limit: pointer(1)})
	if err != nil || len(first.Messages) != 1 || first.NextCursor == nil {
		t.Fatal("first snapshot page failed")
	}
	newMessage, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "new", Text: "new snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{first.Messages[0].ID: true}
	cursor := *first.NextCursor
	for cursor != "" {
		page, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Limit: pointer(1), Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page.Messages {
			if seen[m.ID] || m.ID == newMessage.ID {
				t.Fatal("snapshot duplicated or leaked a new insert")
			}
			seen[m.ID] = true
		}
		cursor = ""
		if page.NextCursor != nil {
			cursor = *page.NextCursor
		}
	}
	if len(seen) != 3 {
		t.Fatal("snapshot skipped an old message")
	}
	fresh, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := mailbox.ToolResult(fresh)
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(result)
	if len(wire) > mailbox.MaxToolResultBytes {
		t.Fatal("result byte limit exceeded")
	}
	if fresh.NextCursor == nil {
		t.Fatal("large page was not bounded with continuation")
	}
	if _, err = f.store.Read(f.ctx, f.ta, mailbox.ReadInput{Cursor: *first.NextCursor}); !errors.Is(err, mailbox.ErrInvalid) {
		t.Fatal("foreign cursor accepted")
	}
	if _, err = f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Cursor: *first.NextCursor, Acknowledged: pointer("all")}); !errors.Is(err, mailbox.ErrInvalid) {
		t.Fatal("changed-filter cursor accepted")
	}
	if _, err = f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Limit: pointer(101)}); !errors.Is(err, mailbox.ErrInvalid) {
		t.Fatal("page cap ignored")
	}
}

func TestMailboxFailureRollsBackMessageThreadAndCounter(t *testing.T) {
	f := mailSetup(t)
	_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION mailbox.reject_message_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic message failure'; END $$;
	CREATE TRIGGER reject_message_test BEFORE INSERT ON mailbox.messages FOR EACH ROW EXECUTE FUNCTION mailbox.reject_message_test();`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "failed", Text: "synthetic"}); !errors.Is(err, mailbox.ErrUnavailable) {
		t.Fatal("failed insertion returned success")
	}
	var messages, threads, counters int
	if f.pool.QueryRow(f.ctx, "SELECT (SELECT count(*) FROM mailbox.messages),(SELECT count(*) FROM mailbox.conversations),(SELECT count(*) FROM mailbox.inbox_counters)").Scan(&messages, &threads, &counters) != nil || messages != 0 || threads != 0 || counters != 0 {
		t.Fatal("partial send persisted")
	}
}

func TestMailboxCompactExtremeMetadataPreservesDomainPrecision(t *testing.T) {
	f := mailSetup(t)
	m, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "domain-number", Text: "synthetic", Metadata: map[string]any{"n": json.Number("1e1000000")}})
	if err != nil || !strings.Contains(string(m.Metadata), "1e1000000") {
		t.Fatalf("compact metadata storage changed: %v", err)
	}
}
