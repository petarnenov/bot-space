package mailbox

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/agents"
)

type Store struct {
	Pool   *pgxpool.Pool
	Auth   *agents.Store
	Signer *Signer
}

func New(pool *pgxpool.Pool, key []byte) (*Store, error) {
	signer, err := NewSigner(key)
	if err != nil {
		return nil, err
	}
	return &Store{Pool: pool, Auth: &agents.Store{Pool: pool}, Signer: signer}, nil
}

const messageColumns = `id::text,workspace_id::text,from_agent_id::text,to_agent_id::text,thread_id::text,in_reply_to::text,kind,text,metadata::text,created_at,acknowledged_at,inbox_sequence`

type scanner interface{ Scan(...any) error }

func scanMessage(row scanner) (Message, error) {
	var m Message
	var metadata string
	err := row.Scan(&m.ID, &m.WorkspaceID, &m.FromAgentID, &m.ToAgentID, &m.ThreadID, &m.InReplyTo, &m.Kind, &m.Text, &metadata, &m.CreatedAt, &m.AcknowledgedAt, &m.Sequence)
	if err != nil {
		return Message{}, err
	}
	m.Metadata = []byte(metadata)
	return m, nil
}

func (s *Store) Send(ctx context.Context, token string, input SendInput) (Message, error) {
	input, metadata, fingerprint, err := normalizeSend(input)
	if err != nil {
		return Message{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Message{}, ErrUnavailable
	}
	defer rollback(tx)
	p, err := s.Auth.AuthenticateTx(ctx, tx, token)
	if err != nil {
		return Message{}, authError(err)
	}
	lockKey := "send:" + p.WorkspaceID + ":" + p.AgentID + ":" + input.IdempotencyKey
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", lockKey); err != nil {
		return Message{}, ErrUnavailable
	}
	var saved string
	err = tx.QueryRow(ctx, "SELECT fingerprint FROM mailbox.messages WHERE workspace_id=$1 AND from_agent_id=$2 AND idempotency_key=$3", p.WorkspaceID, p.AgentID, input.IdempotencyKey).Scan(&saved)
	if err == nil {
		if saved != fingerprint {
			return Message{}, ErrConflict
		}
		m, err := scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM mailbox.messages WHERE workspace_id=$1 AND from_agent_id=$2 AND idempotency_key=$3", p.WorkspaceID, p.AgentID, input.IdempotencyKey))
		if err != nil {
			return Message{}, ErrUnavailable
		}
		return m, commit(ctx, tx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrUnavailable
	}
	if err = recipient(ctx, tx, p.WorkspaceID, input.ToAgentID); err != nil {
		return Message{}, err
	}
	thread, err := conversation(ctx, tx, p, input)
	if err != nil {
		return Message{}, err
	}
	var sequence int64
	err = tx.QueryRow(ctx, `INSERT INTO mailbox.inbox_counters (workspace_id,agent_id,last_sequence) VALUES ($1,$2,1)
		ON CONFLICT (workspace_id,agent_id) DO UPDATE SET last_sequence=mailbox.inbox_counters.last_sequence+1 RETURNING last_sequence`, p.WorkspaceID, input.ToAgentID).Scan(&sequence)
	if err != nil {
		return Message{}, ErrUnavailable
	}
	m, err := scanMessage(tx.QueryRow(ctx, "INSERT INTO mailbox.messages (workspace_id,from_agent_id,to_agent_id,thread_id,in_reply_to,kind,text,metadata,inbox_sequence,idempotency_key,fingerprint) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING "+messageColumns, p.WorkspaceID, p.AgentID, input.ToAgentID, thread, input.InReplyTo, *input.Kind, input.Text, string(metadata), sequence, input.IdempotencyKey, fingerprint))
	if err != nil {
		return Message{}, ErrUnavailable
	}
	return m, commit(ctx, tx)
}

func recipient(ctx context.Context, tx pgx.Tx, workspaceID, agentID string) error {
	var ownerID string
	if err := tx.QueryRow(ctx, "SELECT owner_user_id::text FROM mailbox.agents WHERE workspace_id=$1 AND id=$2", workspaceID, agentID).Scan(&ownerID); err != nil {
		return lookupError(err)
	}
	var role string
	if err := tx.QueryRow(ctx, "SELECT role FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2 FOR SHARE", workspaceID, ownerID).Scan(&role); err != nil {
		return lookupError(err)
	}
	var active bool
	if err := tx.QueryRow(ctx, "SELECT active FROM mailbox.agents WHERE workspace_id=$1 AND id=$2 AND owner_user_id=$3 FOR SHARE", workspaceID, agentID, ownerID).Scan(&active); err != nil {
		return lookupError(err)
	}
	if !active {
		return ErrNotFound
	}
	return nil
}

func conversation(ctx context.Context, tx pgx.Tx, p agents.Principal, input SendInput) (string, error) {
	a, b := p.AgentID, input.ToAgentID
	if a > b {
		a, b = b, a
	}
	thread := ""
	if input.ThreadID != nil {
		thread = *input.ThreadID
	}
	if input.InReplyTo != nil {
		var replyThread string
		if err := tx.QueryRow(ctx, "SELECT thread_id::text FROM mailbox.messages WHERE workspace_id=$1 AND id=$2 AND (from_agent_id=$3 OR to_agent_id=$3)", p.WorkspaceID, *input.InReplyTo, p.AgentID).Scan(&replyThread); err != nil {
			return "", lookupError(err)
		}
		if thread != "" && thread != replyThread {
			return "", ErrNotFound
		}
		thread = replyThread
	}
	if thread != "" {
		var found string
		if err := tx.QueryRow(ctx, "SELECT id::text FROM mailbox.conversations WHERE workspace_id=$1 AND id=$2 AND participant_a=$3 AND participant_b=$4", p.WorkspaceID, thread, a, b).Scan(&found); err != nil {
			return "", lookupError(err)
		}
		return found, nil
	}
	if tx.QueryRow(ctx, "INSERT INTO mailbox.conversations (workspace_id,participant_a,participant_b) VALUES ($1,$2,$3) RETURNING id::text", p.WorkspaceID, a, b).Scan(&thread) != nil {
		return "", ErrUnavailable
	}
	return thread, nil
}

func (s *Store) Whoami(ctx context.Context, token string) (agents.Principal, error) {
	p, err := s.Auth.Authenticate(ctx, token)
	if err != nil {
		return agents.Principal{}, authError(err)
	}
	return p, nil
}

func authError(err error) error {
	if errors.Is(err, agents.ErrUnauthenticated) {
		return ErrForbidden
	}
	return ErrUnavailable
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

func filters(input ReadInput) (CursorScope, int, error) {
	limit, err := pageLimit(input.Limit)
	if err != nil || len(input.Cursor) > MaxCursorBytes {
		return CursorScope{}, 0, ErrInvalid
	}
	state := "unacknowledged"
	if input.Acknowledged != nil {
		state = *input.Acknowledged
	}
	if state != "unacknowledged" && state != "acknowledged" && state != "all" {
		return CursorScope{}, 0, ErrInvalid
	}
	normalizedThread, err := normalizeID(input.ThreadID)
	if err != nil {
		return CursorScope{}, 0, ErrInvalid
	}
	thread := ""
	if normalizedThread != nil {
		thread = *normalizedThread
	}
	kind := ""
	if input.Kind != nil {
		kind = *input.Kind
		if !kindPattern.MatchString(kind) {
			return CursorScope{}, 0, ErrInvalid
		}
	}
	return CursorScope{Purpose: "inbox", Acknowledged: state, Thread: thread, Kind: kind}, limit, nil
}

func canonicalID(value string) string { return strings.ToLower(value) }

func lookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return ErrUnavailable
}
