// Package agents manages workspace-owned agents and separate revocable credentials.
package agents

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden       = errors.New("forbidden")
	ErrNotFound        = errors.New("not_found")
	ErrInvalid         = errors.New("invalid_argument")
	ErrConflict        = errors.New("conflict")
	ErrUnavailable     = errors.New("temporarily_unavailable")
	ErrUnauthenticated = errors.New("unauthenticated")
)

const OperationTimeout = 5 * time.Second

type Store struct{ Pool *pgxpool.Pool }
type Agent struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	OwnerUserID string    `json:"owner_user_id"`
	Name        string    `json:"name"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) Register(ctx context.Context, workspaceID, actorID, name string) (Agent, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || len(name) < 1 || len(name) > 128 || strings.ContainsFunc(name, unicode.IsControl) {
		return Agent{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Agent{}, ErrUnavailable
	}
	defer rollback(tx)
	if _, err = lockActor(ctx, tx, workspaceID, actorID); err != nil {
		return Agent{}, err
	}
	var a Agent
	err = tx.QueryRow(ctx, "INSERT INTO mailbox.agents (workspace_id,owner_user_id,name) VALUES ($1,$2,$3) RETURNING id::text,workspace_id::text,owner_user_id::text,name,active,created_at", workspaceID, actorID, name).Scan(&a.ID, &a.WorkspaceID, &a.OwnerUserID, &a.Name, &a.Active, &a.CreatedAt)
	if err != nil {
		return Agent{}, ErrUnavailable
	}
	if err = audit(ctx, tx, workspaceID, actorID, "agent_registered", a.ID, nil); err != nil {
		return Agent{}, err
	}
	return a, commit(ctx, tx)
}

func (s *Store) Own(ctx context.Context, workspaceID, actorID string) ([]Agent, error) {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rollback(tx)
	if _, err = lockActor(ctx, tx, workspaceID, actorID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT id::text,workspace_id::text,owner_user_id::text,name,active,created_at FROM mailbox.agents WHERE workspace_id=$1 AND owner_user_id=$2 ORDER BY created_at,id LIMIT 100", workspaceID, actorID)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Agent{}
	for rows.Next() {
		var a Agent
		if rows.Scan(&a.ID, &a.WorkspaceID, &a.OwnerUserID, &a.Name, &a.Active, &a.CreatedAt) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, a)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	rows.Close()
	return out, commit(ctx, tx)
}

func (s *Store) Deactivate(ctx context.Context, workspaceID, actorID, agentID string) error {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer rollback(tx)
	role, err := lockActor(ctx, tx, workspaceID, actorID)
	if err != nil {
		return err
	}
	a, err := lockAgent(ctx, tx, workspaceID, agentID)
	if err != nil {
		return err
	}
	if a.OwnerUserID != actorID && role != "owner" && role != "admin" {
		return ErrForbidden
	}
	if a.Active {
		if _, err = tx.Exec(ctx, "UPDATE mailbox.agents SET active=false WHERE workspace_id=$1 AND id=$2", workspaceID, agentID); err != nil {
			return ErrUnavailable
		}
		if err = audit(ctx, tx, workspaceID, actorID, "agent_deactivated", agentID, nil); err != nil {
			return err
		}
	}
	if err = revokeAll(ctx, tx, workspaceID, actorID, agentID); err != nil {
		return err
	}
	return commit(ctx, tx)
}

func lockActor(ctx context.Context, tx pgx.Tx, workspaceID, actorID string) (string, error) {
	var id string
	if tx.QueryRow(ctx, "SELECT id::text FROM mailbox.workspaces WHERE id=$1 FOR UPDATE", workspaceID).Scan(&id) != nil {
		return "", ErrNotFound
	}
	var role string
	if tx.QueryRow(ctx, "SELECT role FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2 FOR SHARE", workspaceID, actorID).Scan(&role) != nil {
		return "", ErrForbidden
	}
	return role, nil
}

func lockAgent(ctx context.Context, tx pgx.Tx, workspaceID, agentID string) (Agent, error) {
	var a Agent
	err := tx.QueryRow(ctx, "SELECT id::text,workspace_id::text,owner_user_id::text,name,active,created_at FROM mailbox.agents WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspaceID, agentID).Scan(&a.ID, &a.WorkspaceID, &a.OwnerUserID, &a.Name, &a.Active, &a.CreatedAt)
	if err != nil {
		return Agent{}, ErrNotFound
	}
	return a, nil
}

func audit(ctx context.Context, tx pgx.Tx, workspaceID, actorID, action, targetID string, metadata map[string]string) error {
	if metadata == nil {
		metadata = map[string]string{}
	}
	_, err := tx.Exec(ctx, "INSERT INTO mailbox.audit_events (workspace_id,actor_kind,actor_user_id,action,target_id,metadata) VALUES ($1,'human',$2,$3,$4,$5)", workspaceID, actorID, action, targetID, metadata)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func commit(ctx context.Context, tx pgx.Tx) error {
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
