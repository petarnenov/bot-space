package integration

import (
	"errors"
	"testing"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/controlevents"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
)

func TestControlOutboxCommitOrderingReplayAndExactAcknowledgement(t *testing.T) {
	ctx, pool, _, workspace, _ := teams(t)
	var project, runner string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, workspace.ID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	secret, _ := security.Secret()
	token := runneridentity.TokenPrefix + secret
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.project_runners(project_id,owner_github_id,public_key,role,credential_hash,credential_expires_at) VALUES($1,101,decode(repeat('ab',32),'hex'),'executor',$2,clock_timestamp()+interval '15 minutes') RETURNING id::text`, project, security.Hash(token)).Scan(&runner); err != nil {
		t.Fatal(err)
	}
	identities := &runneridentity.Store{Pool: pool, Authority: &runnerAuthority{allowed: map[int64]bool{101: true}}}
	store := &controlevents.Store{Pool: pool, Identities: identities}
	frame := func(request string) *pb.ServerFrame {
		return &pb.ServerFrame{Body: &pb.ServerFrame_Receipt{Receipt: &pb.Receipt{RequestId: request}}}
	}
	publish := func(key string) *controlevents.Event {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		event, err := controlevents.PublishTx(ctx, tx, project, runner, key, frame(key))
		if err != nil || tx.Commit(ctx) != nil {
			t.Fatal("publication failed", err)
		}
		return &event
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controlevents.PublishTx(ctx, tx, project, runner, "rolled-back", frame("rollback")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	first := publish("first")
	if first.Sequence != 1 {
		t.Fatal("rollback created sequence gap")
	}
	same := publish("first")
	if same.ID != first.ID || same.Sequence != 1 {
		t.Fatal("delivery retry created event")
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controlevents.PublishTx(ctx, tx, project, runner, "first", frame("changed")); !errors.Is(err, controlevents.ErrConflict) {
		t.Fatal("changed event key accepted", err)
	}
	tx.Rollback(ctx)
	type result struct {
		event controlevents.Event
		err   error
	}
	results := make(chan result, 2)
	for _, key := range []string{"second", "third"} {
		go func(key string) {
			tx, e := pool.Begin(ctx)
			if e != nil {
				results <- result{err: e}
				return
			}
			defer tx.Rollback(ctx)
			event, e := controlevents.PublishTx(ctx, tx, project, runner, key, frame(key))
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- result{event, e}
		}(key)
	}
	a, b := <-results, <-results
	if a.err != nil || b.err != nil || a.event.Sequence == b.event.Sequence {
		t.Fatal("concurrent sequence collision", a.err, b.err)
	}
	page, err := store.Page(ctx, token)
	if err != nil || len(page) != 3 || page[0].ID != first.ID || page[1].Sequence != 2 || page[2].Sequence != 3 {
		t.Fatal("commit order not replayable", err)
	}
	restarted := &controlevents.Store{Pool: pool, Identities: identities}
	replay, err := restarted.Page(ctx, token)
	if err != nil || len(replay) != 3 {
		t.Fatal("sending acknowledged delivery", err)
	}
	if err = restarted.Acknowledge(ctx, token, 2, page[1].ID); !errors.Is(err, controlevents.ErrInvalid) {
		t.Fatal("ACK skipped first event", err)
	}
	if err = restarted.Acknowledge(ctx, token, 1, page[1].ID); !errors.Is(err, controlevents.ErrInvalid) {
		t.Fatal("ACK UUID mismatch accepted", err)
	}
	if err = restarted.Acknowledge(ctx, token, 1, first.ID); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Acknowledge(ctx, token, 1, first.ID); err != nil {
		t.Fatal("duplicate ACK rejected", err)
	}
	remaining, err := store.Page(ctx, token)
	if err != nil || len(remaining) != 2 || remaining[0].Sequence != 2 {
		t.Fatal("ACK cursor not durable", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM mailbox.control_outbox WHERE id=$1`, first.ID); err == nil {
		t.Fatal("event history deleted")
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.project_runners SET active=false WHERE id=$1`, runner); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Page(ctx, token); !errors.Is(err, controlevents.ErrForbidden) {
		t.Fatal("revoked runner replayed events", err)
	}
}
