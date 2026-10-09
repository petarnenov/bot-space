package tasks

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/security"
)

func (s *Store) Heartbeat(ctx context.Context, token, runnerID string) (Ownership, error) {
	if !security.ValidUUID(runnerID) {
		return Ownership{}, ErrInvalid
	}
	runnerID = strings.ToLower(runnerID)
	var out Ownership
	err := s.transaction(ctx, token, func(ctx context.Context, tx pgx.Tx, p agents.Principal, now time.Time) error {
		var previous string
		var generation int64
		var until time.Time
		e := tx.QueryRow(ctx, "SELECT runner_id::text,generation,lease_until FROM mailbox.runner_ownership WHERE workspace_id=$1 AND agent_id=$2 FOR UPDATE", p.WorkspaceID, p.AgentID).Scan(&previous, &generation, &until)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return ErrUnavailable
		}
		if e == nil && until.After(now) && previous != runnerID {
			return ErrLease
		}
		if e != nil || !until.After(now) {
			generation++
		}
		out = Ownership{RunnerID: runnerID, Generation: generation, LeaseUntil: now.Add(LeaseDuration)}
		_, e = tx.Exec(ctx, "INSERT INTO mailbox.runner_ownership (workspace_id,agent_id,runner_id,generation,lease_until,last_seen_at) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (workspace_id,agent_id) DO UPDATE SET runner_id=$3,generation=$4,lease_until=$5,last_seen_at=$6", p.WorkspaceID, p.AgentID, runnerID, generation, out.LeaseUntil, now)
		if e != nil {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return Ownership{}, err
	}
	return out, nil
}

func ownership(ctx context.Context, tx pgx.Tx, p agents.Principal, runnerID string, generation int64, now time.Time) error {
	var found bool
	e := tx.QueryRow(ctx, "SELECT runner_id=$3 AND generation=$4 AND lease_until>$5 FROM mailbox.runner_ownership WHERE workspace_id=$1 AND agent_id=$2 FOR UPDATE", p.WorkspaceID, p.AgentID, runnerID, generation, now).Scan(&found)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !found {
		return ErrLease
	}
	if e != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) Claim(ctx context.Context, token, runnerID string, runnerGeneration int64) (*Task, error) {
	if !security.ValidUUID(runnerID) || runnerGeneration < 1 {
		return nil, ErrInvalid
	}
	runnerID = strings.ToLower(runnerID)
	var out *Task
	err := s.transaction(ctx, token, func(ctx context.Context, tx pgx.Tx, p agents.Principal, now time.Time) error {
		if e := ownership(ctx, tx, p, runnerID, runnerGeneration, now); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND to_agent_id=$2 AND status IN ('queued','running','waiting_dependency') ORDER BY created_at,id LIMIT 100 FOR UPDATE", p.WorkspaceID, p.AgentID)
		if e != nil {
			return ErrUnavailable
		}
		var list []Task
		for rows.Next() {
			t, e := scan(rows)
			if e != nil {
				rows.Close()
				return e
			}
			list = append(list, t)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return ErrUnavailable
		}
		for i := range list {
			if e = s.reconcile(ctx, tx, p, &list[i], now); e != nil {
				return e
			}
			if list[i].Status == "running" || list[i].Status == "waiting_dependency" {
				return nil
			}
		}
		for _, t := range list {
			if t.Status != "queued" {
				continue
			}
			until := now.Add(LeaseDuration)
			if t.Deadline.Before(until) {
				until = t.Deadline
			}
			next, e := scan(tx.QueryRow(ctx, "UPDATE mailbox.tasks SET status='running',generation=generation+1,runner_id=$3,runner_generation=$4,lease_until=$5,updated_at=$6 WHERE workspace_id=$1 AND id=$2 RETURNING "+columns, p.WorkspaceID, t.ID, runnerID, runnerGeneration, until, now))
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, "INSERT INTO mailbox.task_attempts (workspace_id,task_id,generation,runner_id,runner_generation,status,started_at) VALUES ($1,$2,$3,$4,$5,'running',$6)", p.WorkspaceID, t.ID, next.Generation, runnerID, runnerGeneration, now); e != nil {
				return ErrUnavailable
			}
			if e = audit(ctx, tx, p, t.ID, "task_claimed"); e != nil {
				return e
			}
			out = &next
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func checkAttempt(t Task, p agents.Principal, runnerID string, generation, runnerGeneration int64, now time.Time) error {
	if t.ToAgentID != p.AgentID {
		return ErrNotFound
	}
	if (t.Status != "running" && t.Status != "waiting_dependency") || t.Generation != generation || t.RunnerID == nil || *t.RunnerID != runnerID || t.RunnerGeneration == nil || *t.RunnerGeneration != runnerGeneration || t.LeaseUntil == nil || !t.LeaseUntil.After(now) || !t.Deadline.After(now) {
		return ErrLease
	}
	return nil
}

func (s *Store) Renew(ctx context.Context, token, id, runnerID string, generation, runnerGeneration int64) (Task, error) {
	if !security.ValidUUID(id) || !security.ValidUUID(runnerID) || generation < 1 || runnerGeneration < 1 {
		return Task{}, ErrInvalid
	}
	runnerID = strings.ToLower(runnerID)
	var out Task
	denied := false
	err := s.transaction(ctx, token, func(ctx context.Context, tx pgx.Tx, p agents.Principal, now time.Time) error {
		if e := ownership(ctx, tx, p, runnerID, runnerGeneration, now); e != nil {
			return e
		}
		t, e := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM mailbox.tasks WHERE workspace_id=$1 AND id=$2 AND to_agent_id=$3 FOR UPDATE", p.WorkspaceID, id, p.AgentID))
		if e != nil {
			return e
		}
		if e = s.reconcile(ctx, tx, p, &t, now); e != nil {
			return e
		}
		if t.Status != "running" && t.Status != "waiting_dependency" {
			denied = true
			return nil
		}
		if e = checkAttempt(t, p, runnerID, generation, runnerGeneration, now); e != nil {
			return e
		}
		until := now.Add(LeaseDuration)
		if t.Deadline.Before(until) {
			until = t.Deadline
		}
		out, e = scan(tx.QueryRow(ctx, "UPDATE mailbox.tasks SET lease_until=$3,updated_at=$4 WHERE workspace_id=$1 AND id=$2 RETURNING "+columns, p.WorkspaceID, t.ID, until, now))
		return e
	})
	if err != nil {
		return Task{}, err
	}
	if denied {
		return Task{}, ErrLease
	}
	return out, nil
}
