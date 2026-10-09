package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
)

type backlogAuthority struct{ allowed bool }

func (a *backlogAuthority) Verify(context.Context, repositoryaccess.Repository, int64, bool) error {
	if a.allowed {
		return nil
	}
	return repositoryaccess.ErrDenied
}
func TestHumanBacklogDurableIdempotencyAndAuthentication(t *testing.T) {
	ctx, pool, _, w, owner := teams(t)
	var project string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, w.ID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	sessions := &identity.Sessions{Pool: pool}
	_, secret, err := sessions.Login(ctx, owner.GitHubID, owner.Username, "")
	if err != nil {
		t.Fatal(err)
	}
	authority := &backlogAuthority{true}
	store := &backlog.Store{Pool: pool, Sessions: sessions, Authority: authority}
	input := backlog.Input{ProjectID: project, Key: "story-1", Title: "Implement export", Description: "Export verified records", Ticket: "story-123", Priority: 2}
	first, err := store.Create(ctx, secret, input)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &backlog.Store{Pool: pool, Sessions: sessions, Authority: authority}
	same, err := restarted.Create(ctx, secret, input)
	if err != nil || same.ID != first.ID || same.Creator != owner.ID || same.Revision != 1 {
		t.Fatal("restart lost immutable root", err)
	}
	changed := input
	changed.Description = "Different objective"
	if _, err = store.Create(ctx, secret, changed); !errors.Is(err, backlog.ErrConflict) {
		t.Fatal("changed idempotency payload accepted", err)
	}
	if _, err = store.Create(ctx, "bsr_machine-credential", input); !errors.Is(err, backlog.ErrForbidden) {
		t.Fatal("machine impersonated human", err)
	}
	authority.allowed = false
	if _, err = store.Create(ctx, secret, input); !errors.Is(err, backlog.ErrForbidden) {
		t.Fatal("removed project access accepted", err)
	}
	authority.allowed = true
	if err = sessions.Logout(ctx, secret); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Create(ctx, secret, input); !errors.Is(err, backlog.ErrForbidden) {
		t.Fatal("revoked human session accepted", err)
	}
	var roots, audits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.human_intentions WHERE project_id=$1", project).Scan(&roots); err != nil || roots != 1 {
		t.Fatal("duplicate roots", err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.audit_events WHERE action='intention.created' AND target_id=$1", first.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("duplicate or missing audit", err)
	}
}
