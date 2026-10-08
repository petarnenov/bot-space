package workspaces

import (
	"context"
	"time"

	"github.com/petarnenov/bot-space/internal/security"
)

const InvitationLifetime = 48 * time.Hour

type Invitation struct {
	ID, WorkspaceID, Role   string
	TargetGitHubID          int64
	ExpiresAt               time.Time
	AcceptedAt, CancelledAt *time.Time
}

func (s *Store) Invite(ctx context.Context, workspaceID, actorID string, targetGitHubID int64, role string) (Invitation, string, error) {
	if targetGitHubID < 1 || (role != "member" && role != "admin") {
		return Invitation{}, "", ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Invitation{}, "", ErrUnavailable
	}
	defer rollback(tx)
	actor, err := lockActor(ctx, tx, workspaceID, actorID)
	if err != nil {
		return Invitation{}, "", err
	}
	if actor != "owner" && actor != "admin" {
		return Invitation{}, "", ErrForbidden
	}
	var already bool
	if tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM mailbox.memberships m JOIN mailbox.users u ON u.id=m.user_id WHERE m.workspace_id=$1 AND u.github_id=$2)", workspaceID, targetGitHubID).Scan(&already) != nil {
		return Invitation{}, "", ErrUnavailable
	}
	if already {
		return Invitation{}, "", ErrConflict
	}
	secret, err := security.Secret()
	if err != nil {
		return Invitation{}, "", ErrUnavailable
	}
	var invite Invitation
	err = tx.QueryRow(ctx, "INSERT INTO mailbox.invitations (workspace_id,target_github_id,role,secret_hash,created_by,expires_at) VALUES ($1,$2,$3,$4,$5,now()+interval '48 hours') RETURNING id::text,workspace_id::text,target_github_id,role,expires_at", workspaceID, targetGitHubID, role, security.Hash(secret), actorID).Scan(&invite.ID, &invite.WorkspaceID, &invite.TargetGitHubID, &invite.Role, &invite.ExpiresAt)
	if err != nil {
		return Invitation{}, "", ErrUnavailable
	}
	if err = audit(ctx, tx, workspaceID, actorID, "invitation_created", invite.ID); err != nil {
		return Invitation{}, "", err
	}
	if err = commit(ctx, tx); err != nil {
		return Invitation{}, "", err
	}
	return invite, secret, nil
}

func (s *Store) Invitations(ctx context.Context, workspaceID, actorID string) ([]Invitation, error) {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rollback(tx)
	actor, err := lockActor(ctx, tx, workspaceID, actorID)
	if err != nil {
		return nil, err
	}
	if actor != "owner" && actor != "admin" {
		return nil, ErrForbidden
	}
	rows, err := tx.Query(ctx, "SELECT id::text,workspace_id::text,target_github_id,role,expires_at,accepted_at,cancelled_at FROM mailbox.invitations WHERE workspace_id=$1 ORDER BY created_at DESC,id LIMIT 100", workspaceID)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Invitation{}
	for rows.Next() {
		var i Invitation
		if rows.Scan(&i.ID, &i.WorkspaceID, &i.TargetGitHubID, &i.Role, &i.ExpiresAt, &i.AcceptedAt, &i.CancelledAt) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, i)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	rows.Close()
	return out, commit(ctx, tx)
}

func (s *Store) CancelInvitation(ctx context.Context, workspaceID, actorID, inviteID string) error {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer rollback(tx)
	actor, err := lockActor(ctx, tx, workspaceID, actorID)
	if err != nil {
		return err
	}
	if actor != "owner" && actor != "admin" {
		return ErrForbidden
	}
	var accepted, cancelled *time.Time
	if tx.QueryRow(ctx, "SELECT accepted_at,cancelled_at FROM mailbox.invitations WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspaceID, inviteID).Scan(&accepted, &cancelled) != nil {
		return ErrNotFound
	}
	if cancelled != nil {
		return commit(ctx, tx)
	}
	if accepted != nil {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, "UPDATE mailbox.invitations SET cancelled_at=now() WHERE workspace_id=$1 AND id=$2", workspaceID, inviteID); err != nil {
		return ErrUnavailable
	}
	if err = audit(ctx, tx, workspaceID, actorID, "invitation_cancelled", inviteID); err != nil {
		return err
	}
	return commit(ctx, tx)
}

func (s *Store) AcceptInvitation(ctx context.Context, actorID, inviteID, secret string) (Workspace, error) {
	if len(secret) != 43 {
		return Workspace{}, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Workspace{}, ErrUnavailable
	}
	defer rollback(tx)
	var workspaceID string
	err = tx.QueryRow(ctx, "SELECT i.workspace_id::text FROM mailbox.invitations i JOIN mailbox.users u ON u.github_id=i.target_github_id WHERE i.id=$1 AND u.id=$2 AND i.secret_hash=$3", inviteID, actorID, security.Hash(secret)).Scan(&workspaceID)
	if err != nil {
		return Workspace{}, ErrForbidden
	}
	var w Workspace
	if tx.QueryRow(ctx, "SELECT id::text,slug FROM mailbox.workspaces WHERE id=$1 FOR UPDATE", workspaceID).Scan(&w.ID, &w.Slug) != nil {
		return Workspace{}, ErrNotFound
	}
	var valid bool
	err = tx.QueryRow(ctx, "SELECT role,expires_at>now() AND accepted_at IS NULL AND cancelled_at IS NULL FROM mailbox.invitations WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspaceID, inviteID).Scan(&w.Role, &valid)
	if err != nil || !valid {
		return Workspace{}, ErrForbidden
	}
	// Recheck wall-clock expiry after acquiring the row lock, rather than using
	// the transaction's start timestamp when another mutation held the lock.
	if tx.QueryRow(ctx, "SELECT expires_at>clock_timestamp() FROM mailbox.invitations WHERE workspace_id=$1 AND id=$2", workspaceID, inviteID).Scan(&valid) != nil || !valid {
		return Workspace{}, ErrForbidden
	}
	var already bool
	if tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2)", workspaceID, actorID).Scan(&already) != nil {
		return Workspace{}, ErrUnavailable
	}
	if already {
		return Workspace{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, "INSERT INTO mailbox.memberships (workspace_id,user_id,role) VALUES ($1,$2,$3)", workspaceID, actorID, w.Role); err != nil {
		return Workspace{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "UPDATE mailbox.invitations SET accepted_at=now() WHERE workspace_id=$1 AND id=$2", workspaceID, inviteID); err != nil {
		return Workspace{}, ErrUnavailable
	}
	if err = audit(ctx, tx, workspaceID, actorID, "invitation_accepted", inviteID); err != nil {
		return Workspace{}, err
	}
	if err = commit(ctx, tx); err != nil {
		return Workspace{}, err
	}
	return w, nil
}
