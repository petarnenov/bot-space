package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

func user(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64) workspaces.User {
	t.Helper()
	session, _, err := (&identity.Sessions{Pool: pool}).Login(ctx, id, "synthetic-user", "")
	if err != nil {
		t.Fatal(err)
	}
	return session.User
}

func teams(t *testing.T) (context.Context, *pgxpool.Pool, *workspaces.Store, workspaces.Workspace, workspaces.User) {
	t.Helper()
	ctx, url, pool := isolated(t)
	if err := database.Migrate(ctx, url, bundle(t)); err != nil {
		t.Fatal(err)
	}
	s := &workspaces.Store{Pool: pool}
	w, err := s.Bootstrap(ctx, 101, "team-one")
	if err != nil {
		t.Fatal(err)
	}
	return ctx, pool, s, w, user(t, ctx, pool, 101)
}

func join(t *testing.T, ctx context.Context, pool *pgxpool.Pool, s *workspaces.Store, w workspaces.Workspace, owner workspaces.User, id int64, role string) workspaces.User {
	t.Helper()
	target := user(t, ctx, pool, id)
	i, secret, err := s.Invite(ctx, w.ID, owner.ID, id, role)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptInvitation(ctx, target.ID, i.ID, secret); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestWorkspaceIsolationBootstrapAndRoles(t *testing.T) {
	ctx, pool, s, w, owner := teams(t)
	admin := join(t, ctx, pool, s, w, owner, 102, "admin")
	member := join(t, ctx, pool, s, w, owner, 103, "member")
	other, err := s.Bootstrap(ctx, 201, "team-two")
	if err != nil {
		t.Fatal(err)
	}
	outsider := user(t, ctx, pool, 201)
	if repeated, err := s.Bootstrap(ctx, 101, "team-one"); err != nil || repeated.ID != w.ID {
		t.Fatal("bootstrap not idempotent")
	}
	if _, err := s.Bootstrap(ctx, 201, "team-one"); !errors.Is(err, workspaces.ErrConflict) {
		t.Fatal("conflicting bootstrap allowed")
	}
	if _, err := s.Members(ctx, w.ID, outsider.ID); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("foreign members exposed")
	}
	if _, err := s.Members(ctx, other.ID, owner.ID); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("cross-team members exposed")
	}
	if err := s.SetRole(ctx, w.ID, admin.ID, admin.ID, "owner"); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("admin escalated")
	}
	if err := s.Remove(ctx, w.ID, admin.ID, owner.ID); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("admin removed owner")
	}
	if err := s.SetRole(ctx, w.ID, admin.ID, owner.ID, "member"); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("admin demoted owner")
	}
	if err := s.SetRole(ctx, w.ID, member.ID, admin.ID, "member"); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("member changed roles")
	}
	if err := s.Remove(ctx, w.ID, member.ID, admin.ID); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("member removed another member")
	}
	if err := s.Remove(ctx, w.ID, owner.ID, owner.ID); !errors.Is(err, workspaces.ErrConflict) {
		t.Fatal("last owner removed")
	}
	if err := s.SetRole(ctx, w.ID, owner.ID, owner.ID, "admin"); !errors.Is(err, workspaces.ErrConflict) {
		t.Fatal("last owner demoted")
	}
	// The same identity may join another workspace only through a targeted invite.
	i, secret, err := s.Invite(ctx, other.ID, outsider.ID, owner.GitHubID, "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptInvitation(ctx, owner.ID, i.ID, secret); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, owner.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("multiple memberships: %v", err)
	}
	if err = s.SetRole(ctx, w.ID, owner.ID, member.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if err = s.Remove(ctx, w.ID, owner.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	var roleChanges, removed, bootstrap int
	if err = pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE action='member_role_changed' AND metadata->>'role_to'='admin'),count(*) FILTER (WHERE action='member_removed'),count(*) FILTER (WHERE action='workspace_bootstrapped' AND actor_kind='system' AND actor_user_id IS NULL) FROM mailbox.audit_events WHERE workspace_id=$1", w.ID).Scan(&roleChanges, &removed, &bootstrap); err != nil || roleChanges != 1 || removed != 1 || bootstrap != 1 {
		t.Fatal("audit mismatch")
	}
}

func TestLastOwnerConcurrency(t *testing.T) {
	ctx, pool, s, w, owner := teams(t)
	second := join(t, ctx, pool, s, w, owner, 102, "admin")
	if err := s.SetRole(ctx, w.ID, owner.ID, second.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, target := range []workspaces.User{owner, second} {
		wg.Add(1)
		go func(target workspaces.User) {
			defer wg.Done()
			errs <- s.SetRole(ctx, w.ID, target.ID, target.ID, "member")
		}(target)
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, workspaces.ErrConflict) {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.memberships WHERE workspace_id=$1 AND role='owner'", w.ID).Scan(&count); err != nil || count != 1 || successes != 1 {
		t.Fatal("last-owner invariant violated")
	}
}

func TestInvitationsIdentityExpiryCancellationAndAudit(t *testing.T) {
	ctx, pool, s, w, owner := teams(t)
	target := user(t, ctx, pool, 102)
	wrong := user(t, ctx, pool, 103)
	i, secret, err := s.Invite(ctx, w.ID, owner.ID, 102, "member")
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Until(i.ExpiresAt); elapsed < 47*time.Hour || elapsed > 49*time.Hour {
		t.Fatal("invalid expiry")
	}
	var hash string
	if pool.QueryRow(ctx, "SELECT secret_hash FROM mailbox.invitations WHERE workspace_id=$1 AND id=$2", w.ID, i.ID).Scan(&hash) != nil || hash != security.Hash(secret) || hash == secret {
		t.Fatal("secret not hashed")
	}
	if _, err = s.AcceptInvitation(ctx, wrong.ID, i.ID, secret); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("forwarded invite accepted")
	}
	if _, err = s.AcceptInvitation(ctx, target.ID, i.ID, "wrong-token"); err == nil {
		t.Fatal("wrong token accepted")
	}
	wrongToken, err := security.Secret()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptInvitation(ctx, target.ID, i.ID, wrongToken); err == nil {
		t.Fatal("wrong equal-length secret accepted")
	}
	if _, err = s.AcceptInvitation(ctx, target.ID, i.ID, secret); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptInvitation(ctx, target.ID, i.ID, secret); err == nil {
		t.Fatal("replay accepted")
	}
	if _, _, err = s.Invite(ctx, w.ID, owner.ID, 102, "member"); !errors.Is(err, workspaces.ErrConflict) {
		t.Fatal("current member invited")
	}
	if _, _, err = s.Invite(ctx, w.ID, target.ID, 104, "member"); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("member invited")
	}
	for _, role := range []string{"owner", "invalid"} {
		if _, _, err = s.Invite(ctx, w.ID, owner.ID, 104, role); !errors.Is(err, workspaces.ErrInvalid) {
			t.Fatal("invalid invitation role accepted")
		}
	}
	if _, _, err = s.Invite(ctx, w.ID, owner.ID, 0, "member"); !errors.Is(err, workspaces.ErrInvalid) {
		t.Fatal("invalid ID accepted")
	}
	expired, expiredSecret, err := s.Invite(ctx, w.ID, owner.ID, 103, "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE mailbox.invitations SET created_at=now()-interval '3 days',expires_at=now()-interval '1 day' WHERE id=$1", expired.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptInvitation(ctx, wrong.ID, expired.ID, expiredSecret); err == nil {
		t.Fatal("expired accepted")
	}
	cancelled, cancelledSecret, err := s.Invite(ctx, w.ID, owner.ID, 103, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelInvitation(ctx, w.ID, target.ID, cancelled.ID); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("member cancelled invite")
	}
	for j := 0; j < 2; j++ {
		if err = s.CancelInvitation(ctx, w.ID, owner.ID, cancelled.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.AcceptInvitation(ctx, wrong.ID, cancelled.ID, cancelledSecret); err == nil {
		t.Fatal("cancelled accepted")
	}
	if _, err = s.Invitations(ctx, w.ID, wrong.ID); !errors.Is(err, workspaces.ErrForbidden) {
		t.Fatal("foreign invitation listing")
	}
	other, err := s.Bootstrap(ctx, 103, "other-team")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelInvitation(ctx, other.ID, wrong.ID, i.ID); !errors.Is(err, workspaces.ErrNotFound) {
		t.Fatal("foreign invitation cancelled through another workspace")
	}
	var created, accepted, cancellations int
	if pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE action='invitation_created'),count(*) FILTER (WHERE action='invitation_accepted'),count(*) FILTER (WHERE action='invitation_cancelled') FROM mailbox.audit_events WHERE workspace_id=$1", w.ID).Scan(&created, &accepted, &cancellations) != nil || created != 3 || accepted != 1 || cancellations != 1 {
		t.Fatal("invitation audit mismatch")
	}
	var secretPresent bool
	if pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM mailbox.audit_events WHERE workspace_id=$1 AND row_to_json(audit_events)::text LIKE $2)", w.ID, "%"+secret+"%").Scan(&secretPresent) != nil || secretPresent {
		t.Fatal("secret in audit")
	}
}

func TestInvitationConcurrentAcceptance(t *testing.T) {
	ctx, pool, s, w, owner := teams(t)
	target := user(t, ctx, pool, 102)
	i, secret, err := s.Invite(ctx, w.ID, owner.ID, 102, "member")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for j := 0; j < 2; j++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.AcceptInvitation(ctx, target.ID, i.ID, secret); errs <- err }()
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful accepts: %d", successes)
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2", w.ID, target.ID).Scan(&count) != nil || count != 1 {
		t.Fatal("duplicate membership")
	}
}

func TestMembershipMutationRollsBackWhenAuditFails(t *testing.T) {
	ctx, pool, s, w, owner := teams(t)
	member := join(t, ctx, pool, s, w, owner, 102, "member")
	_, err := pool.Exec(ctx, `CREATE FUNCTION mailbox.reject_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END $$;
	CREATE TRIGGER reject_test_audit BEFORE INSERT ON mailbox.audit_events FOR EACH ROW EXECUTE FUNCTION mailbox.reject_test_audit();`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRole(ctx, w.ID, owner.ID, member.ID, "admin"); !errors.Is(err, workspaces.ErrUnavailable) {
		t.Fatal("audit failure did not fail mutation")
	}
	var role string
	if pool.QueryRow(ctx, "SELECT role FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2", w.ID, member.ID).Scan(&role) != nil || role != "member" {
		t.Fatal("role mutation committed without audit")
	}
}
