package tasks

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/security"
)

type ParticipantTask struct {
	Task
	FromAgentName    string `json:"from_agent_name"`
	ToAgentName      string `json:"to_agent_name"`
	FromOwnerUserID  string `json:"from_owner_user_id"`
	ToOwnerUserID    string `json:"to_owner_user_id"`
	RecipientOffline bool   `json:"recipient_offline"`
}

const participantColumns = "t.id::text,t.workspace_id::text,t.from_agent_id::text,t.to_agent_id::text,t.parent_task_id::text,t.root_task_id::text,t.depth,t.instruction,t.retry_safe,t.status,t.deadline,t.created_at,t.generation,t.lease_until,t.result,t.error_code,t.request_message_id::text,t.reply_message_id::text,t.runner_id::text,t.runner_generation,t.ancestor_agents::text[],sender.name,recipient.name,sender.owner_user_id::text,recipient.owner_user_id::text,(owner.lease_until IS NULL OR owner.lease_until<=clock_timestamp() OR owner.last_seen_at<=clock_timestamp()-interval '90 seconds')"

func scanParticipant(row interface{ Scan(...any) error }) (ParticipantTask, error) {
	var task ParticipantTask
	err := row.Scan(&task.ID, &task.WorkspaceID, &task.FromAgentID, &task.ToAgentID, &task.ParentTaskID, &task.RootTaskID, &task.Depth, &task.Instruction, &task.RetrySafe, &task.Status, &task.Deadline, &task.CreatedAt, &task.Generation, &task.LeaseUntil, &task.Result, &task.ErrorCode, &task.RequestMessageID, &task.ReplyMessageID, &task.RunnerID, &task.RunnerGeneration, &task.Ancestors, &task.FromAgentName, &task.ToAgentName, &task.FromOwnerUserID, &task.ToOwnerUserID, &task.RecipientOffline)
	if errors.Is(err, pgx.ErrNoRows) {
		return ParticipantTask{}, ErrNotFound
	}
	if err != nil {
		return ParticipantTask{}, ErrUnavailable
	}
	return task, nil
}

func (s *Store) ListForParticipant(ctx context.Context, workspaceID, userID, after string) ([]ParticipantTask, error) {
	if !security.ValidUUID(workspaceID) || !security.ValidUUID(userID) || (after != "" && !security.ValidUUID(after)) {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.Pool.Query(ctx, `SELECT `+participantColumns+`
FROM mailbox.tasks t
JOIN mailbox.memberships member ON member.workspace_id=t.workspace_id AND member.user_id=$2
JOIN mailbox.agents sender ON sender.workspace_id=t.workspace_id AND sender.id=t.from_agent_id
JOIN mailbox.agents recipient ON recipient.workspace_id=t.workspace_id AND recipient.id=t.to_agent_id
LEFT JOIN mailbox.runner_ownership owner ON owner.workspace_id=t.workspace_id AND owner.agent_id=t.to_agent_id
WHERE t.workspace_id=$1
  AND (sender.owner_user_id=$2 OR recipient.owner_user_id=$2)
  AND ($3='' OR (t.created_at,t.id)>(SELECT created_at,id FROM mailbox.tasks WHERE workspace_id=$1 AND id=NULLIF($3,'')::uuid))
ORDER BY t.created_at,t.id
LIMIT 50`, workspaceID, userID, after)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	var out []ParticipantTask
	for rows.Next() {
		task, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

func (s *Store) GetForParticipant(ctx context.Context, workspaceID, userID, taskID string) (ParticipantTask, error) {
	if !security.ValidUUID(workspaceID) || !security.ValidUUID(userID) || !security.ValidUUID(taskID) {
		return ParticipantTask{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return scanParticipant(s.Pool.QueryRow(ctx, `SELECT `+participantColumns+`
FROM mailbox.tasks t
JOIN mailbox.memberships member ON member.workspace_id=t.workspace_id AND member.user_id=$2
JOIN mailbox.agents sender ON sender.workspace_id=t.workspace_id AND sender.id=t.from_agent_id
JOIN mailbox.agents recipient ON recipient.workspace_id=t.workspace_id AND recipient.id=t.to_agent_id
LEFT JOIN mailbox.runner_ownership owner ON owner.workspace_id=t.workspace_id AND owner.agent_id=t.to_agent_id
WHERE t.workspace_id=$1 AND t.id=$3 AND (sender.owner_user_id=$2 OR recipient.owner_user_id=$2)`, workspaceID, userID, taskID))
}

func (s *Store) CancelForParticipant(ctx context.Context, workspaceID, userID, taskID string) (Task, error) {
	if !security.ValidUUID(workspaceID) || !security.ValidUUID(userID) || !security.ValidUUID(taskID) {
		return Task{}, ErrInvalid
	}
	var out Task
	err := s.transactionAsParticipant(ctx, workspaceID, userID, func(ctx context.Context, tx pgx.Tx, principal agents.Principal, now time.Time) error {
		task, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspaceID, taskID))
		if err != nil {
			return err
		}
		var senderOwner string
		if tx.QueryRow(ctx, "SELECT owner_user_id::text FROM mailbox.agents WHERE workspace_id=$1 AND id=$2", workspaceID, task.FromAgentID).Scan(&senderOwner) != nil || senderOwner != userID {
			return ErrForbidden
		}
		if terminal(task.Status) {
			out = task
			return nil
		}
		rows, err := tx.Query(ctx, `WITH RECURSIVE descendants AS (
  SELECT id FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2
  UNION ALL
  SELECT child.id FROM mailbox.tasks child JOIN descendants parent ON child.parent_task_id=parent.id WHERE child.workspace_id=$1
) SELECT `+columns+` FROM mailbox.tasks WHERE workspace_id=$1 AND id IN (SELECT id FROM descendants) FOR UPDATE`, workspaceID, taskID)
		if err != nil {
			return ErrUnavailable
		}
		var descendants []Task
		for rows.Next() {
			child, err := scan(rows)
			if err != nil {
				rows.Close()
				return err
			}
			descendants = append(descendants, child)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return ErrUnavailable
		}
		for i := range descendants {
			if !terminal(descendants[i].Status) {
				if err = s.transition(ctx, tx, principal, &descendants[i], "cancelled", "task_cancelled", now); err != nil {
					return err
				}
			}
			if descendants[i].ID == taskID {
				out = descendants[i]
			}
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return out, nil
}

func (s *Store) ReviewForParticipant(ctx context.Context, workspaceID, userID, taskID, action string) (Task, error) {
	if !security.ValidUUID(workspaceID) || !security.ValidUUID(userID) || !security.ValidUUID(taskID) {
		return Task{}, ErrInvalid
	}
	if action != "retry" {
		return Task{}, ErrInvalid
	}
	var out Task
	err := s.transactionAsParticipant(ctx, workspaceID, userID, func(ctx context.Context, tx pgx.Tx, principal agents.Principal, now time.Time) error {
		task, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspaceID, taskID))
		if err != nil {
			return err
		}
		var owners [2]string
		rows, err := tx.Query(ctx, "SELECT owner_user_id::text FROM mailbox.agents WHERE workspace_id=$1 AND id=ANY($2::uuid[])", workspaceID, []string{task.FromAgentID, task.ToAgentID})
		if err != nil {
			return ErrUnavailable
		}
		i := 0
		for rows.Next() && i < 2 {
			if rows.Scan(&owners[i]) != nil {
				rows.Close()
				return ErrUnavailable
			}
			i++
		}
		rows.Close()
		if owners[0] != userID && owners[1] != userID {
			return ErrForbidden
		}
		if task.Status != "requires_approval" && task.Status != "interrupted" && task.Status != "failed" {
			return ErrConflict
		}
		out, err = scan(tx.QueryRow(ctx, "UPDATE mailbox.tasks SET status='queued',error_code=NULL,runner_id=NULL,runner_generation=NULL,lease_until=NULL,updated_at=$3 WHERE workspace_id=$1 AND id=$2 RETURNING "+columns, workspaceID, taskID, now))
		if err != nil {
			return err
		}
		return audit(ctx, tx, principal, taskID, "task_review_retry")
	})
	if err != nil {
		return Task{}, err
	}
	return out, nil
}

func (s *Store) transactionAsParticipant(ctx context.Context, workspaceID, userID string, action func(context.Context, pgx.Tx, agents.Principal, time.Time) error) error {
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
	var member string
	if tx.QueryRow(ctx, "SELECT user_id::text FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2", workspaceID, userID).Scan(&member) != nil {
		return ErrForbidden
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "tasks:"+workspaceID); err != nil {
		return ErrUnavailable
	}
	var now time.Time
	if tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now) != nil {
		return ErrUnavailable
	}
	principal := agents.Principal{WorkspaceID: workspaceID, OwnerUserID: userID}
	if err = action(ctx, tx, principal, now); err != nil {
		return err
	}
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
