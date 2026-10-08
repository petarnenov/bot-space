package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/mailbox"
)

func TestMailboxCounterWaitsForEarlierCommit(t *testing.T) {
	f := mailSetup(t)
	observer, err := pgx.Connect(f.ctx, f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(context.Background())
	const gate int64 = 871204907
	_, err = f.pool.Exec(f.ctx, `CREATE FUNCTION mailbox.delay_first_send_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.idempotency_key='delayed' THEN PERFORM pg_advisory_xact_lock(871204907); END IF; RETURN NEW; END $$;
	CREATE TRIGGER delay_first_send_test BEFORE INSERT ON mailbox.messages FOR EACH ROW EXECUTE FUNCTION mailbox.delay_first_send_test();`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = observer.Exec(f.ctx, "SELECT pg_advisory_lock($1)", gate); err != nil {
		t.Fatal(err)
	}
	defer observer.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", gate)
	type outcome struct {
		message mailbox.Message
		err     error
	}
	first := make(chan outcome, 1)
	second := make(chan outcome, 1)
	go func() {
		m, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "delayed", Text: "first"})
		first <- outcome{m, err}
	}()
	waitLock := func(pattern string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			var waiting bool
			if observer.QueryRow(f.ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1)", pattern).Scan(&waiting) != nil {
				t.Fatal("lock observation failed")
			}
			if waiting {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("expected transaction lock wait not observed")
	}
	waitLock("INSERT INTO mailbox.messages%")
	go func() {
		m, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "later", Text: "second"})
		second <- outcome{m, err}
	}()
	waitLock("INSERT INTO mailbox.inbox_counters%")
	if _, err = observer.Exec(f.ctx, "SELECT pg_advisory_unlock($1)", gate); err != nil {
		t.Fatal(err)
	}
	a, b := <-first, <-second
	if a.err != nil || b.err != nil || a.message.Sequence != 1 || b.message.Sequence != 2 {
		t.Fatalf("commit ordering failed: %v %v", a.err, b.err)
	}
	page, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Limit: pointer(1)})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != a.message.ID || page.NextCursor == nil {
		t.Fatal("first commit skipped")
	}
	page, err = f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Limit: pointer(1), Cursor: *page.NextCursor})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != b.message.ID {
		t.Fatal("later commit skipped")
	}
}

func TestMailboxLiveAcknowledgementFilterAndSharedIdentity(t *testing.T) {
	f := mailSetup(t)
	var messages []mailbox.Message
	for _, key := range []string{"one", "two", "three"} {
		m, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: key, Text: key})
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, m)
	}
	first, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Limit: pointer(1)})
	if err != nil || first.NextCursor == nil {
		t.Fatal(err)
	}
	if _, err = f.store.Acknowledge(f.ctx, f.tb, messages[1].ID); err != nil {
		t.Fatal(err)
	}
	continued, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Limit: pointer(1), Cursor: *first.NextCursor})
	if err != nil || len(continued.Messages) != 1 || continued.Messages[0].ID != messages[2].ID {
		t.Fatal("live acknowledgement filter ignored")
	}
	acknowledged, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{Acknowledged: pointer("acknowledged")})
	if err != nil || len(acknowledged.Messages) != 1 || acknowledged.Messages[0].ID != messages[1].ID {
		t.Fatal("acknowledged history unavailable")
	}
	srv := mcpHTTP(t, f)
	one := native(t, f, srv, f.tb)
	two := native(t, f, srv, f.tb)
	var a, b mailbox.Page
	if one.Call(f.ctx, "read_messages", map[string]any{}, &a) != nil || two.Call(f.ctx, "read_messages", map[string]any{}, &b) != nil || len(a.Messages) != 2 || len(b.Messages) != 2 || a.Messages[0].ID != b.Messages[0].ID {
		t.Fatal("shared identity did not permit repeated delivery")
	}
}
