package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/security"
)

type Store struct {
	Pool    *pgxpool.Pool
	Auth    *agents.Store
	Mailbox *mailbox.Store
}

func New(m *mailbox.Store) *Store { return &Store{Pool: m.Pool, Auth: m.Auth, Mailbox: m} }

const columns = "id::text,workspace_id::text,from_agent_id::text,to_agent_id::text,parent_task_id::text,root_task_id::text,depth,instruction,retry_safe,status,deadline,created_at,generation,lease_until,result,error_code,request_message_id::text,reply_message_id::text,runner_id::text,runner_generation,ancestor_agents::text[]"

func scan(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.WorkspaceID, &t.FromAgentID, &t.ToAgentID, &t.ParentTaskID, &t.RootTaskID, &t.Depth, &t.Instruction, &t.RetrySafe, &t.Status, &t.Deadline, &t.CreatedAt, &t.Generation, &t.LeaseUntil, &t.Result, &t.ErrorCode, &t.RequestMessageID, &t.ReplyMessageID, &t.RunnerID, &t.RunnerGeneration, &t.Ancestors)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, ErrUnavailable
	}
	return t, nil
}

func (s *Store) transaction(ctx context.Context, token string, action func(context.Context, pgx.Tx, agents.Principal, time.Time) error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(c)
	}()
	p, err := s.Auth.AuthenticateTx(ctx, tx, token)
	if errors.Is(err, agents.ErrUnauthenticated) {
		return ErrForbidden
	}
	if err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "tasks:"+p.WorkspaceID); err != nil {
		return ErrUnavailable
	}
	var now time.Time
	if tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now) != nil {
		return ErrUnavailable
	}
	if err = action(ctx, tx, p, now); err != nil {
		return err
	}
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}

func validText(text string, max int, empty bool) bool {
	return (empty || text != "") && len(text) <= max && utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}

func normalize(input SubmitInput) (SubmitInput, string, error) {
	if !security.ValidUUID(input.ToAgentID) || !validText(input.Instruction, MaxInstructionBytes, false) || len(input.IdempotencyKey) < 1 || len(input.IdempotencyKey) > 128 {
		return SubmitInput{}, "", ErrInvalid
	}
	for _, c := range []byte(input.IdempotencyKey) {
		if c < 32 || c > 126 {
			return SubmitInput{}, "", ErrInvalid
		}
	}
	input.ToAgentID = strings.ToLower(input.ToAgentID)
	if input.ParentTaskID != nil {
		if !security.ValidUUID(*input.ParentTaskID) {
			return SubmitInput{}, "", ErrInvalid
		}
		id := strings.ToLower(*input.ParentTaskID)
		input.ParentTaskID = &id
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if input.TimeoutSeconds < 1 || input.TimeoutSeconds > MaxTimeoutSeconds || input.ParentGeneration < 0 {
		return SubmitInput{}, "", ErrInvalid
	}
	if (input.ParentTaskID == nil && input.ParentGeneration != 0) || (input.ParentTaskID != nil && input.ParentGeneration < 1) {
		return SubmitInput{}, "", ErrInvalid
	}
	body, _ := json.Marshal(input)
	h := sha256.Sum256(body)
	return input, hex.EncodeToString(h[:]), nil
}

func audit(ctx context.Context, tx pgx.Tx, p agents.Principal, id, action string) error {
	_, err := tx.Exec(ctx, "INSERT INTO mailbox.audit_events (workspace_id,actor_kind,actor_user_id,action,target_id,metadata) VALUES ($1,'agent',$2,$3,$4,'{}')", p.WorkspaceID, p.OwnerUserID, action, id)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) Submit(ctx context.Context, token string, input SubmitInput) (Task, error) {
	input, fingerprint, err := normalize(input)
	if err != nil {
		return Task{}, err
	}
	var out Task
	err = s.transaction(ctx, token, func(ctx context.Context, tx pgx.Tx, p agents.Principal, now time.Time) error {
		var existing string
		e := tx.QueryRow(ctx, "SELECT fingerprint FROM mailbox.tasks WHERE workspace_id=$1 AND from_agent_id=$2 AND idempotency_key=$3", p.WorkspaceID, p.AgentID, input.IdempotencyKey).Scan(&existing)
		if e == nil {
			if existing != fingerprint {
				return ErrConflict
			}
			out, e = scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND from_agent_id=$2 AND idempotency_key=$3", p.WorkspaceID, p.AgentID, input.IdempotencyKey))
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return ErrUnavailable
		}
		if input.ToAgentID == p.AgentID {
			return ErrDependency
		}
		var id string
		if tx.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&id) != nil {
			return ErrUnavailable
		}
		root, depth, ancestors := id, 1, []string{p.AgentID}
		deadline := now.Add(time.Duration(input.TimeoutSeconds) * time.Second)
		if input.ParentTaskID != nil {
			parent, e := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2 AND to_agent_id=$3 FOR UPDATE", p.WorkspaceID, *input.ParentTaskID, p.AgentID))
			if e != nil {
				return e
			}
			if (parent.Status != "running" && parent.Status != "waiting_dependency") || parent.Generation != input.ParentGeneration || parent.LeaseUntil == nil || !parent.LeaseUntil.After(now) || !parent.Deadline.After(now) {
				return ErrLease
			}
			if parent.Depth >= 4 {
				return ErrDependency
			}
			for _, ancestor := range parent.Ancestors {
				if ancestor == input.ToAgentID {
					return ErrDependency
				}
			}
			var children int
			if tx.QueryRow(ctx, "SELECT count(*) FROM mailbox.tasks WHERE workspace_id=$1 AND parent_task_id=$2", p.WorkspaceID, parent.ID).Scan(&children) != nil {
				return ErrUnavailable
			}
			if children >= 4 {
				return ErrDependency
			}
			root, depth = parent.RootTaskID, parent.Depth+1
			ancestors = append(append([]string{}, parent.Ancestors...), p.AgentID)
			if parent.Deadline.Before(deadline) {
				deadline = parent.Deadline
			}
			if _, e = tx.Exec(ctx, "UPDATE mailbox.tasks SET status='waiting_dependency',updated_at=$3 WHERE workspace_id=$1 AND id=$2", p.WorkspaceID, parent.ID, now); e != nil {
				return ErrUnavailable
			}
		}
		kind := "task.request"
		msg, e := s.Mailbox.SendTx(ctx, tx, p, mailbox.SendInput{ToAgentID: input.ToAgentID, IdempotencyKey: "task-request:" + id, Text: input.Instruction, Kind: &kind, Metadata: map[string]any{"task_id": id}})
		if e != nil {
			return mapMailboxError(e)
		}
		out, e = scan(tx.QueryRow(ctx, "INSERT INTO mailbox.tasks (id,workspace_id,from_agent_id,to_agent_id,parent_task_id,root_task_id,depth,ancestor_agents,idempotency_key,fingerprint,instruction,retry_safe,deadline,created_at,updated_at,request_message_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14,$15) RETURNING "+columns, id, p.WorkspaceID, p.AgentID, input.ToAgentID, input.ParentTaskID, root, depth, ancestors, input.IdempotencyKey, fingerprint, input.Instruction, input.RetrySafe, deadline, now, msg.ID))
		if e != nil {
			return e
		}
		return audit(ctx, tx, p, id, "task_submitted")
	})
	if err != nil {
		return Task{}, err
	}
	return out, nil
}

func mapMailboxError(err error) error {
	switch {
	case errors.Is(err, mailbox.ErrInvalid):
		return ErrInvalid
	case errors.Is(err, mailbox.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, mailbox.ErrForbidden):
		return ErrForbidden
	case errors.Is(err, mailbox.ErrConflict):
		return ErrConflict
	}
	return ErrUnavailable
}

func (s *Store) Get(ctx context.Context, token, id string) (Task, error) {
	if !security.ValidUUID(id) {
		return Task{}, ErrInvalid
	}
	var out Task
	err := s.transaction(ctx, token, func(ctx context.Context, tx pgx.Tx, p agents.Principal, now time.Time) error {
		t, e := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2 AND (from_agent_id=$3 OR to_agent_id=$3) FOR UPDATE", p.WorkspaceID, id, p.AgentID))
		if e != nil {
			return e
		}
		if e = s.reconcile(ctx, tx, p, &t, now); e != nil {
			return e
		}
		out = t
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return out, nil
}
