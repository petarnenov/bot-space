package integration

import (
	"errors"
	"testing"

	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/identity"
)

func TestHumanLifecycleFencingHistoryAndRestart(t *testing.T) {
	ctx, pool, _, workspace, owner := teams(t)
	var project string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, workspace.ID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	sessions := &identity.Sessions{Pool: pool}
	_, secret, err := sessions.Login(ctx, owner.GitHubID, owner.Username, "")
	if err != nil {
		t.Fatal(err)
	}
	authority := &backlogAuthority{true}
	store := &backlog.Store{Pool: pool, Sessions: sessions, Authority: authority}
	root, err := store.Create(ctx, secret, backlog.Input{ProjectID: project, Key: "lifecycle", Title: "Durable objective", Description: "Preserve all evidence"})
	if err != nil {
		t.Fatal(err)
	}
	checkFence := func(epoch int64, want bool) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		err = backlog.LockExecutable(ctx, tx, project, root.ID, root.Revision, epoch)
		if (err == nil) != want {
			t.Fatalf("execution fence epoch %d: %v", epoch, err)
		}
	}
	checkFence(1, true)
	_, otherSecret, err := sessions.Login(ctx, 202, "another-collaborator", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Control(ctx, otherSecret, project, root.ID, "pause", 1); !errors.Is(err, backlog.ErrForbidden) {
		t.Fatal("other collaborator controlled root", err)
	}
	if _, err = store.Control(ctx, "bsr_machine", project, root.ID, "pause", 1); !errors.Is(err, backlog.ErrForbidden) {
		t.Fatal("machine controlled root", err)
	}
	authority.allowed = false
	if _, err = store.Control(ctx, secret, project, root.ID, "pause", 1); !errors.Is(err, backlog.ErrForbidden) {
		t.Fatal("removed collaborator controlled root", err)
	}
	authority.allowed = true
	type result struct {
		item backlog.Intention
		err  error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			item, err := store.Control(ctx, secret, project, root.ID, "pause", 1)
			results <- result{item, err}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil {
		first, second = second, first
	}
	if first.err != nil || !errors.Is(second.err, backlog.ErrConflict) || first.item.State != "paused" || first.item.Epoch != 2 {
		t.Fatal("concurrent pause not serialized", first.err, second.err)
	}
	checkFence(1, false)
	checkFence(2, false)
	restarted := &backlog.Store{Pool: pool, Sessions: sessions, Authority: authority}
	resumed, err := restarted.Control(ctx, secret, project, root.ID, "resume", 2)
	if err != nil || resumed.State != "blocked" || resumed.Epoch != 3 || !resumed.ReconciliationRequired {
		t.Fatal("resume bypassed reconciliation", err)
	}
	checkFence(3, false)
	cancelled, err := restarted.Control(ctx, secret, project, root.ID, "cancel", 3)
	if err != nil || cancelled.State != "cancelled" || cancelled.Epoch != 4 {
		t.Fatal("cancel failed", err)
	}
	for _, action := range []string{"resume", "pause", "cancel"} {
		if _, err = restarted.Control(ctx, secret, project, root.ID, action, 4); !errors.Is(err, backlog.ErrConflict) {
			t.Fatal("cancelled root revived", action, err)
		}
	}
	archived, err := restarted.Control(ctx, secret, project, root.ID, "archive", 4)
	if err != nil || !archived.Archived || archived.Epoch != 5 {
		t.Fatal("archive failed", err)
	}
	page, err := restarted.List(ctx, secret, project, "")
	if err != nil || len(page) != 0 {
		t.Fatal("archive still visible in active list", err)
	}
	original, err := restarted.Get(ctx, secret, project, root.ID, 1)
	if err != nil || original.Description != root.Description || !original.Archived {
		t.Fatal("archive lost original evidence", err)
	}
	history, err := restarted.History(ctx, secret, project, root.ID)
	if err != nil || len(history) != 4 {
		t.Fatal("history lost after restart", err)
	}
	for i, event := range history {
		if event.Epoch != int64(i+2) || event.Actor != owner.ID {
			t.Fatal("history provenance lost", event)
		}
	}
	if _, err = pool.Exec(ctx, `DELETE FROM mailbox.human_intention_lifecycle WHERE intention_id=$1`, root.ID); err == nil {
		t.Fatal("history deletion permitted")
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.human_intention_lifecycle SET action='resume' WHERE intention_id=$1`, root.ID); err == nil {
		t.Fatal("history rewrite permitted")
	}
	if _, err = restarted.Revise(ctx, secret, root.ID, 1, backlog.Input{ProjectID: project, Key: "revision", Title: "Revive", Description: "Bad revival"}); err == nil {
		t.Fatal("cancelled archived revision permitted")
	}
}
