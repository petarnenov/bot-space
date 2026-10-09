package backlog

import (
	"context"
	"time"

	"fmt"
	"github.com/jackc/pgx/v5"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/controlevents"
	"github.com/petarnenov/bot-space/internal/security"
)

type LifecycleEvent struct {
	Epoch                            int64
	Actor, Action, Previous, State   string
	Archived, ReconciliationRequired bool
	CreatedAt                        time.Time
}

// Control serializes creator actions against the exact displayed lifecycle.
// A resumed root remains fenced until the council reconciles its effects.
func (s *Store) Control(ctx context.Context, secret, project, id, action string, expected int64) (Intention, error) {
	if !security.ValidUUID(project) || !security.ValidUUID(id) || expected < 1 {
		return Intention{}, ErrInvalid
	}
	switch action {
	case "pause", "resume", "cancel", "archive":
	default:
		return Intention{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	human, workspace, repo, err := s.authorize(ctx, secret, project)
	if err != nil {
		return Intention{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	defer rollback(tx)
	var user string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM mailbox.sessions WHERE id_hash=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>clock_timestamp() AND last_seen_at>clock_timestamp()-interval '30 minutes' FOR SHARE`, security.Hash(secret), human.User.ID).Scan(&user)
	if err != nil {
		return Intention{}, ErrForbidden
	}
	var creator, state string
	var epoch int64
	var revision int
	var archived, reconcile bool
	err = tx.QueryRow(ctx, `SELECT i.creator_user_id::text,i.state,i.lifecycle_epoch,i.current_revision,i.archived,i.reconciliation_required
 FROM mailbox.human_intentions i JOIN mailbox.orchestration_projects p ON p.id=i.project_id
 WHERE i.project_id=$1 AND i.id=$2 AND p.active AND p.repository_id=$3 AND p.repository_owner_id=$4 AND p.repository_owner=$5 AND p.repository_name=$6
 FOR UPDATE OF i FOR SHARE OF p`, project, id, repo.ID, repo.OwnerID, repo.Owner, repo.Name).Scan(&creator, &state, &epoch, &revision, &archived, &reconcile)
	if err != nil || creator != human.User.ID {
		return Intention{}, ErrForbidden
	}
	if epoch != expected {
		return Intention{}, ErrConflict
	}
	previous := state
	switch action {
	case "pause":
		if archived || state == "paused" || state == "completed" || state == "cancelled" {
			return Intention{}, ErrConflict
		}
		state = "paused"
		reconcile = true
	case "resume":
		if archived || state != "paused" {
			return Intention{}, ErrConflict
		}
		state = "blocked"
		reconcile = true
	case "cancel":
		if archived || state == "cancelled" || state == "completed" {
			return Intention{}, ErrConflict
		}
		state = "cancelled"
		reconcile = true
	case "archive":
		if archived || (state != "paused" && state != "cancelled" && state != "completed") {
			return Intention{}, ErrConflict
		}
		archived = true
	}
	epoch++
	_, err = tx.Exec(ctx, `UPDATE mailbox.human_intentions SET state=$3,lifecycle_epoch=$4,archived=$5,reconciliation_required=$6 WHERE project_id=$1 AND id=$2`, project, id, state, epoch, archived, reconcile)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.human_intention_lifecycle(project_id,intention_id,epoch,actor_user_id,action,previous_state,resulting_state,archived,reconciliation_required) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, project, id, epoch, human.User.ID, action, previous, state, archived, reconcile)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.audit_events(workspace_id,actor_kind,actor_user_id,action,target_id,metadata) VALUES($1,'human',$2,$3,$4,jsonb_build_object('project_id',$5::text,'epoch',$6::bigint,'previous_state',$7::text,'state',$8::text))`, workspace, human.User.ID, "intention."+action, id, project, epoch, previous, state)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	out, err := read(ctx, tx, project, id, revision)
	if err != nil {
		return Intention{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM mailbox.project_runners WHERE project_id=$1 AND active ORDER BY id`, project)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	targets := []string{}
	for rows.Next() {
		var target string
		if rows.Scan(&target) != nil {
			rows.Close()
			return Intention{}, ErrUnavailable
		}
		targets = append(targets, target)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	for _, target := range targets {
		_, err = controlevents.PublishTx(ctx, tx, project, target, fmt.Sprintf("root:%s:%d", id, epoch), &pb.ServerFrame{Body: &pb.ServerFrame_RootControl{RootControl: &pb.RootControl{RootId: id, LifecycleEpoch: uint64(epoch), Action: action, State: state}}})
		if err != nil {
			return Intention{}, ErrUnavailable
		}
	}
	if tx.Commit(ctx) != nil {
		return Intention{}, ErrUnavailable
	}
	return out, nil
}

func (s *Store) History(ctx context.Context, secret, project, id string) ([]LifecycleEvent, error) {
	if !security.ValidUUID(project) || !security.ValidUUID(id) {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, _, _, err := s.authorize(ctx, secret, project); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT epoch,actor_user_id::text,action,previous_state,resulting_state,archived,reconciliation_required,created_at FROM mailbox.human_intention_lifecycle WHERE project_id=$1 AND intention_id=$2 ORDER BY epoch`, project, id)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []LifecycleEvent{}
	for rows.Next() {
		var event LifecycleEvent
		if err = rows.Scan(&event.Epoch, &event.Actor, &event.Action, &event.Previous, &event.State, &event.Archived, &event.ReconciliationRequired, &event.CreatedAt); err != nil {
			return nil, ErrUnavailable
		}
		out = append(out, event)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

// LockExecutable holds a shared root lock until the caller commits its derived
// mutation. Human control takes an exclusive lock, invalidating older epochs.
// Callers must check current runner/council authority in the same transaction.
func LockExecutable(ctx context.Context, tx pgx.Tx, project, id string, revision int, epoch int64) error {
	var found string
	err := tx.QueryRow(ctx, `SELECT id::text FROM mailbox.human_intentions WHERE project_id=$1 AND id=$2 AND current_revision=$3 AND lifecycle_epoch=$4 AND NOT archived AND NOT reconciliation_required AND state NOT IN ('paused','cancelled','completed') FOR SHARE`, project, id, revision, epoch).Scan(&found)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrConflict
		}
		return ErrUnavailable
	}
	return nil
}
