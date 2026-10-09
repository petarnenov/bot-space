package tasks

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/security"
)

var errorPattern = regexp.MustCompile("^[a-z_]{1,64}$")

func terminal(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled" || status == "expired" || status == "interrupted" || status == "requires_approval"
}

func (s *Store) transition(ctx context.Context, tx pgx.Tx, p agents.Principal, t *Task, status, code string, now time.Time) error {
	_, e := tx.Exec(ctx, "UPDATE mailbox.tasks SET status=$3,error_code=$4,updated_at=$5 WHERE workspace_id=$1 AND id=$2", t.WorkspaceID, t.ID, status, code, now)
	if e != nil {
		return ErrUnavailable
	}
	if t.Generation > 0 {
		if _, e = tx.Exec(ctx, "UPDATE mailbox.task_attempts SET status=$4,error_code=$5,finished_at=$6 WHERE workspace_id=$1 AND task_id=$2 AND generation=$3", t.WorkspaceID, t.ID, t.Generation, status, code, now); e != nil {
			return ErrUnavailable
		}
	}
	if e = audit(ctx, tx, p, t.ID, "task_"+status); e != nil {
		return e
	}
	t.Status = status
	t.ErrorCode = &code
	return nil
}

func (s *Store) reconcile(ctx context.Context, tx pgx.Tx, p agents.Principal, t *Task, now time.Time) error {
	if terminal(t.Status) {
		return nil
	}
	for _, id := range []string{t.FromAgentID, t.ToAgentID} {
		var active bool
		err := tx.QueryRow(ctx, "SELECT a.active FROM mailbox.agents a JOIN mailbox.memberships m ON m.workspace_id=a.workspace_id AND m.user_id=a.owner_user_id WHERE a.workspace_id=$1 AND a.id=$2 FOR SHARE OF a,m", t.WorkspaceID, id).Scan(&active)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !active {
			return s.transition(ctx, tx, p, t, "cancelled", "access_terminated", now)
		}
		if err != nil {
			return ErrUnavailable
		}
	}
	if !t.Deadline.After(now) {
		return s.transition(ctx, tx, p, t, "expired", "deadline_exceeded", now)
	}
	if (t.Status == "running" || t.Status == "waiting_dependency") && (t.LeaseUntil == nil || !t.LeaseUntil.After(now)) {
		if e := s.transition(ctx, tx, p, t, "interrupted", "lease_lost", now); e != nil {
			return e
		}
		if t.RetrySafe {
			_, e := tx.Exec(ctx, "UPDATE mailbox.tasks SET status='queued',runner_id=NULL,runner_generation=NULL,lease_until=NULL,error_code=NULL WHERE workspace_id=$1 AND id=$2", t.WorkspaceID, t.ID)
			if e != nil {
				return ErrUnavailable
			}
			t.Status = "queued"
			t.ErrorCode = nil
			t.RunnerID = nil
			t.RunnerGeneration = nil
			t.LeaseUntil = nil
			return audit(ctx, tx, p, t.ID, "task_retry_queued")
		}
	}
	return nil
}

func (s *Store) Complete(ctx context.Context, token string, input CompleteInput) (Task, error) {
	if !security.ValidUUID(input.TaskID) || !security.ValidUUID(input.RunnerID) || input.Generation < 1 || input.RunnerGeneration < 1 || !validText(input.Result, MaxResultBytes, true) || (input.ErrorCode != "" && !errorPattern.MatchString(input.ErrorCode)) {
		return Task{}, ErrInvalid
	}
	input.RunnerID = strings.ToLower(input.RunnerID)
	var out Task
	err := s.transaction(ctx, token, func(ctx context.Context, tx pgx.Tx, p agents.Principal, now time.Time) error {
		t, e := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2 AND to_agent_id=$3 FOR UPDATE", p.WorkspaceID, input.TaskID, p.AgentID))
		if e != nil {
			return e
		}
		if e = ownership(ctx, tx, p, input.RunnerID, input.RunnerGeneration, now); e != nil {
			return e
		}
		// Delivery retries are allowed for the exact already-committed attempt,
		// even though the execution lease no longer needs renewal.
		if (t.Status == "completed" || t.Status == "failed" || t.Status == "requires_approval") && t.Generation == input.Generation && t.RunnerID != nil && *t.RunnerID == input.RunnerID && t.RunnerGeneration != nil && *t.RunnerGeneration == input.RunnerGeneration {
			code := ""
			if t.ErrorCode != nil {
				code = *t.ErrorCode
			}
			if t.Result == nil || *t.Result != input.Result || code != input.ErrorCode {
				return ErrConflict
			}
			out = t
			return nil
		}
		if e = checkAttempt(t, p, input.RunnerID, input.Generation, input.RunnerGeneration, now); e != nil {
			return e
		}
		var pending int
		if tx.QueryRow(ctx, "SELECT count(*) FROM mailbox.tasks WHERE workspace_id=$1 AND parent_task_id=$2 AND status IN ('queued','running','waiting_dependency')", p.WorkspaceID, t.ID).Scan(&pending) != nil {
			return ErrUnavailable
		}
		if pending > 0 {
			return ErrDependency
		}
		status := "completed"
		var code *string
		if input.ErrorCode != "" {
			status = "failed"
			code = &input.ErrorCode
			if input.ErrorCode == "requires_approval" {
				status = "requires_approval"
			}
		}
		kind := "task.result"
		reply, e := s.Mailbox.SendTx(ctx, tx, p, mailbox.SendInput{ToAgentID: t.FromAgentID, IdempotencyKey: "task-result:" + t.ID, Kind: &kind, Text: "Task " + t.ID + " " + status + ". Read the task for its full result.", InReplyTo: &t.RequestMessageID, Metadata: map[string]any{"task_id": t.ID, "status": status}})
		if e != nil {
			return mapMailboxError(e)
		}
		out, e = scan(tx.QueryRow(ctx, "UPDATE mailbox.tasks SET status=$3,result=$4,error_code=$5,reply_message_id=$6,updated_at=$7 WHERE workspace_id=$1 AND id=$2 RETURNING "+columns, p.WorkspaceID, t.ID, status, input.Result, code, reply.ID, now))
		if e != nil {
			return e
		}
		if _, e = mailbox.ToolResult(out); e != nil {
			return ErrInvalid
		}
		if _, e = tx.Exec(ctx, "UPDATE mailbox.task_attempts SET status=$4,finished_at=$5,error_code=$6 WHERE workspace_id=$1 AND task_id=$2 AND generation=$3", p.WorkspaceID, t.ID, input.Generation, status, now, code); e != nil {
			return ErrUnavailable
		}
		if _, e = tx.Exec(ctx, "UPDATE mailbox.messages SET acknowledged_at=COALESCE(acknowledged_at,$4) WHERE workspace_id=$1 AND id=$2 AND to_agent_id=$3", p.WorkspaceID, t.RequestMessageID, p.AgentID, now); e != nil {
			return ErrUnavailable
		}
		if e = audit(ctx, tx, p, t.ID, "task_"+status); e != nil {
			return e
		}
		if t.ParentTaskID != nil {
			_, e = tx.Exec(ctx, "UPDATE mailbox.tasks parent SET status='running',updated_at=$3 WHERE parent.workspace_id=$1 AND parent.id=$2 AND parent.status='waiting_dependency' AND NOT EXISTS (SELECT 1 FROM mailbox.tasks child WHERE child.workspace_id=$1 AND child.parent_task_id=$2 AND child.status IN ('queued','running','waiting_dependency'))", p.WorkspaceID, *t.ParentTaskID, now)
			if e != nil {
				return ErrUnavailable
			}
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return out, nil
}

func (s *Store) Cancel(ctx context.Context, token, id string) (Task, error) {
	if !security.ValidUUID(id) {
		return Task{}, ErrInvalid
	}
	var out Task
	err := s.transaction(ctx, token, func(ctx context.Context, tx pgx.Tx, p agents.Principal, now time.Time) error {
		t, e := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2 AND from_agent_id=$3 FOR UPDATE", p.WorkspaceID, id, p.AgentID))
		if e != nil {
			return e
		}
		if terminal(t.Status) {
			out = t
			return nil
		}
		// The bounded graph is cancelled atomically so child workers observe the
		// same decision on their next renewal and cannot complete late.
		rows, e := tx.Query(ctx, "WITH RECURSIVE descendants AS (SELECT id FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2 UNION ALL SELECT child.id FROM mailbox.tasks child JOIN descendants parent ON child.parent_task_id=parent.id WHERE child.workspace_id=$1) SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND id IN (SELECT id FROM descendants) FOR UPDATE", p.WorkspaceID, id)
		if e != nil {
			return ErrUnavailable
		}
		var list []Task
		for rows.Next() {
			child, e := scan(rows)
			if e != nil {
				rows.Close()
				return e
			}
			list = append(list, child)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return ErrUnavailable
		}
		for i := range list {
			if !terminal(list[i].Status) {
				if e = s.transition(ctx, tx, p, &list[i], "cancelled", "task_cancelled", now); e != nil {
					return e
				}
			}
			if list[i].ID == t.ID {
				out = list[i]
			}
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return out, nil
}
