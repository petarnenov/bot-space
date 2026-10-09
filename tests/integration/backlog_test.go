package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

type backlogAuthority struct{ allowed bool }

func (a *backlogAuthority) Verify(context.Context, repositoryaccess.Repository, int64, bool) error {
	if a.allowed {
		return nil
	}
	return repositoryaccess.ErrDenied
}

type projectAuthority struct {
	allowed map[int64]bool
	err     error
}

func (a *projectAuthority) Verify(_ context.Context, repo repositoryaccess.Repository, _ int64, _ bool) error {
	if a.err != nil {
		return a.err
	}
	if a.allowed[repo.ID] {
		return nil
	}
	return repositoryaccess.ErrDenied
}

func TestProjectDiscoveryUsesRepositoryAuthority(t *testing.T) {
	ctx, pool, _, workspace, owner := teams(t)
	foreign, err := (&workspaces.Store{Pool: pool}).Bootstrap(ctx, 909, "foreign-project-team")
	if err != nil {
		t.Fatal(err)
	}
	var first, second string
	if err = pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','first') RETURNING id::text`, workspace.ID).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,43,909,'foreign','second') RETURNING id::text`, foreign.ID).Scan(&second); err != nil {
		t.Fatal(err)
	}
	sessions := &identity.Sessions{Pool: pool}
	_, secret, err := sessions.Login(ctx, owner.GitHubID, owner.Username, "")
	if err != nil {
		t.Fatal(err)
	}
	authority := &projectAuthority{allowed: map[int64]bool{42: true, 43: true}}
	store := &backlog.Store{Pool: pool, Sessions: sessions, Authority: authority}
	projects, err := store.Projects(ctx, secret)
	if err != nil || len(projects) != 2 {
		t.Fatal("repository authority did not discover projects independently of workspace role", err, len(projects))
	}
	authority.allowed[43] = false
	projects, err = store.Projects(ctx, secret)
	if err != nil || len(projects) != 1 || projects[0].ID != first {
		t.Fatal("revoked project remained discoverable", err, projects)
	}
	if _, err = store.Project(ctx, secret, second); !errors.Is(err, backlog.ErrForbidden) {
		t.Fatal("revoked selected project admitted", err)
	}
	authority.err = repositoryaccess.ErrUnavailable
	if _, err = store.Projects(ctx, secret); !errors.Is(err, backlog.ErrUnavailable) {
		t.Fatal("authority outage was not fail-closed", err)
	}
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
	updated := input
	updated.Description = "Export records with verified filters"
	newer, err := store.Revise(ctx, secret, first.ID, 1, updated)
	if err != nil || newer.Revision != 2 {
		t.Fatal("revision append failed", err)
	}
	original, err := store.Get(ctx, secret, project, first.ID, 1)
	if err != nil || original.Description != input.Description || original.Creator != first.Creator {
		t.Fatal("revision overwrote original provenance", err)
	}
	if _, err = store.Revise(ctx, secret, first.ID, 1, updated); !errors.Is(err, backlog.ErrConflict) {
		t.Fatal("stale revision accepted", err)
	}
	latest, err := store.Get(ctx, secret, project, first.ID, 0)
	if err != nil || latest.Revision != 2 || latest.Description != updated.Description {
		t.Fatal("latest revision missing", err)
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

func TestHumanBacklogConcurrentKeysAndProjectIsolation(t *testing.T) {
	ctx, pool, teamsStore, w, owner := teams(t)
	other, err := teamsStore.Bootstrap(ctx, owner.GitHubID, "other-backlog")
	if err != nil {
		t.Fatal(err)
	}
	var projects [2]string
	for i, workspace := range []string{w.ID, other.ID} {
		if err = pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,$2,101,'owner','project') RETURNING id::text`, workspace, 42+i).Scan(&projects[i]); err != nil {
			t.Fatal(err)
		}
	}
	sessions := &identity.Sessions{Pool: pool}
	_, secret, err := sessions.Login(ctx, owner.GitHubID, owner.Username, "")
	if err != nil {
		t.Fatal(err)
	}
	store := &backlog.Store{Pool: pool, Sessions: sessions, Authority: &backlogAuthority{true}}
	input := backlog.Input{ProjectID: projects[0], Key: "concurrent", Title: "One goal", Description: "One authoritative goal"}
	type result struct {
		item backlog.Intention
		err  error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() { item, err := store.Create(ctx, secret, input); results <- result{item, err} }()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.item.ID != second.item.ID {
		t.Fatal("concurrent intake duplicated root", first.err, second.err)
	}
	if _, err = store.Get(ctx, secret, projects[1], first.item.ID, 1); err == nil {
		t.Fatal("cross-project root read accepted")
	}
	page, err := store.List(ctx, secret, projects[1], first.item.ID)
	if err != nil || len(page) != 0 {
		t.Fatal("foreign cursor leaked another project", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE mailbox.human_intentions SET workspace_id=$1 WHERE id=$2", other.ID, first.item.ID); err == nil {
		t.Fatal("cross-workspace composite reference accepted")
	}
	page, err = store.List(ctx, secret, projects[0], "")
	if err != nil || len(page) != 1 || page[0].ID != first.item.ID {
		t.Fatal("authoritative project backlog missing", err)
	}
}
