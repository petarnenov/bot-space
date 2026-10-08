package agents

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/security"
)

const TokenPrefix = "bot_space_v1_"

type Credential struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}

func validToken(token string) bool {
	return len(token) == len(TokenPrefix)+43 && strings.HasPrefix(token, TokenPrefix)
}

func (s *Store) Issue(ctx context.Context, workspaceID, actorID, agentID string) (Credential, string, error) {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Credential{}, "", ErrUnavailable
	}
	defer rollback(tx)
	if _, err = owned(ctx, tx, workspaceID, actorID, agentID, true); err != nil {
		return Credential{}, "", err
	}
	c, token, err := issue(ctx, tx, workspaceID, actorID, agentID)
	if err != nil {
		return Credential{}, "", err
	}
	if err = commit(ctx, tx); err != nil {
		return Credential{}, "", err
	}
	return c, token, nil
}

func issue(ctx context.Context, tx pgx.Tx, workspaceID, actorID, agentID string) (Credential, string, error) {
	random, err := security.Secret()
	if err != nil {
		return Credential{}, "", ErrUnavailable
	}
	token := TokenPrefix + random
	var c Credential
	err = tx.QueryRow(ctx, "INSERT INTO mailbox.agent_credentials (workspace_id,agent_id,secret_hash) VALUES ($1,$2,$3) RETURNING id::text,agent_id::text,created_at,revoked_at", workspaceID, agentID, security.Hash(token)).Scan(&c.ID, &c.AgentID, &c.CreatedAt, &c.RevokedAt)
	if err != nil {
		return Credential{}, "", ErrUnavailable
	}
	if err = audit(ctx, tx, workspaceID, actorID, "credential_issued", c.ID, map[string]string{"agent_id": agentID}); err != nil {
		return Credential{}, "", err
	}
	return c, token, nil
}

func (s *Store) Credentials(ctx context.Context, workspaceID, actorID, agentID string) ([]Credential, error) {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rollback(tx)
	if _, err = owned(ctx, tx, workspaceID, actorID, agentID, false); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT id::text,agent_id::text,created_at,revoked_at FROM mailbox.agent_credentials WHERE workspace_id=$1 AND agent_id=$2 ORDER BY created_at,id LIMIT 100", workspaceID, agentID)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Credential{}
	for rows.Next() {
		var c Credential
		if rows.Scan(&c.ID, &c.AgentID, &c.CreatedAt, &c.RevokedAt) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, c)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	rows.Close()
	return out, commit(ctx, tx)
}

func (s *Store) Revoke(ctx context.Context, workspaceID, actorID, agentID, credentialID string) error {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer rollback(tx)
	if _, err = owned(ctx, tx, workspaceID, actorID, agentID, false); err != nil {
		return err
	}
	if err = revokeOne(ctx, tx, workspaceID, actorID, agentID, credentialID); err != nil {
		return err
	}
	return commit(ctx, tx)
}

func revokeOne(ctx context.Context, tx pgx.Tx, workspaceID, actorID, agentID, credentialID string) error {
	var revoked *time.Time
	if tx.QueryRow(ctx, "SELECT revoked_at FROM mailbox.agent_credentials WHERE workspace_id=$1 AND agent_id=$2 AND id=$3 FOR UPDATE", workspaceID, agentID, credentialID).Scan(&revoked) != nil {
		return ErrNotFound
	}
	if revoked != nil {
		return nil
	}
	if _, err := tx.Exec(ctx, "UPDATE mailbox.agent_credentials SET revoked_at=clock_timestamp() WHERE workspace_id=$1 AND agent_id=$2 AND id=$3", workspaceID, agentID, credentialID); err != nil {
		return ErrUnavailable
	}
	return audit(ctx, tx, workspaceID, actorID, "credential_revoked", credentialID, map[string]string{"agent_id": agentID})
}

func revokeAll(ctx context.Context, tx pgx.Tx, workspaceID, actorID, agentID string) error {
	rows, err := tx.Query(ctx, "UPDATE mailbox.agent_credentials SET revoked_at=clock_timestamp() WHERE workspace_id=$1 AND agent_id=$2 AND revoked_at IS NULL RETURNING id::text", workspaceID, agentID)
	if err != nil {
		return ErrUnavailable
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return ErrUnavailable
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		rows.Close()
		return ErrUnavailable
	}
	rows.Close()
	for _, id := range ids {
		if err = audit(ctx, tx, workspaceID, actorID, "credential_revoked", id, map[string]string{"agent_id": agentID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Rotate(ctx context.Context, workspaceID, actorID, agentID, credentialID string) (Credential, string, error) {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Credential{}, "", ErrUnavailable
	}
	defer rollback(tx)
	if _, err = owned(ctx, tx, workspaceID, actorID, agentID, true); err != nil {
		return Credential{}, "", err
	}
	var active bool
	if tx.QueryRow(ctx, "SELECT revoked_at IS NULL FROM mailbox.agent_credentials WHERE workspace_id=$1 AND agent_id=$2 AND id=$3 FOR UPDATE", workspaceID, agentID, credentialID).Scan(&active) != nil {
		return Credential{}, "", ErrNotFound
	}
	if !active {
		return Credential{}, "", ErrConflict
	}
	if err = revokeOne(ctx, tx, workspaceID, actorID, agentID, credentialID); err != nil {
		return Credential{}, "", err
	}
	c, token, err := issue(ctx, tx, workspaceID, actorID, agentID)
	if err != nil {
		return Credential{}, "", err
	}
	if err = audit(ctx, tx, workspaceID, actorID, "credential_rotated", credentialID, map[string]string{"replacement_id": c.ID, "agent_id": agentID}); err != nil {
		return Credential{}, "", err
	}
	if err = commit(ctx, tx); err != nil {
		return Credential{}, "", err
	}
	return c, token, nil
}

func owned(ctx context.Context, tx pgx.Tx, workspaceID, actorID, agentID string, requireActive bool) (Agent, error) {
	if _, err := lockActor(ctx, tx, workspaceID, actorID); err != nil {
		return Agent{}, err
	}
	a, err := lockAgent(ctx, tx, workspaceID, agentID)
	if err != nil {
		return Agent{}, err
	}
	if a.OwnerUserID != actorID {
		return Agent{}, ErrForbidden
	}
	if requireActive && !a.Active {
		return Agent{}, ErrConflict
	}
	return a, nil
}
