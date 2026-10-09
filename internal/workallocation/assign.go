package workallocation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/controlevents"
	"github.com/petarnenov/bot-space/internal/councilstore"
	"github.com/petarnenov/bot-space/internal/security"
)

func machine(owner int64, key []byte) string {
	return fmt.Sprintf("executor-slot:%d:%s", owner, hex.EncodeToString(key))
}

func (s *Store) Assign(ctx context.Context, token, decision string) (Assignment, error) {
	if !security.ValidUUID(decision) {
		return Assignment{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token, pb.Role_ROLE_ARCHITECT)
	if err != nil {
		return Assignment{}, err
	}
	council := &councilstore.Store{Pool: s.Pool, Identities: s.Identities}
	record, err := council.Get(ctx, token, decision)
	if err != nil {
		return Assignment{}, err
	}
	if record.Project != p.ProjectID || record.Snapshot.Kind != "allocation" || !strings.HasPrefix(record.Subject, "task:") {
		return Assignment{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Assignment{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Assignment{}, err
	}
	record, err = councilstore.LockAccepted(ctx, tx, p.ProjectID, decision, "allocation", record.Subject, record.Material.SpecDigest)
	if err != nil {
		return Assignment{}, err
	}
	round := record.Snapshot.Rounds[len(record.Snapshot.Rounds)-1]
	if len(round.Proposal.Actions) != 1 {
		return Assignment{}, ErrInvalid
	}
	action := round.Proposal.Actions[0]
	task := strings.TrimPrefix(record.Subject, "task:")
	if action.Kind != "assign" || action.Value != task || !security.ValidUUID(action.Target) {
		return Assignment{}, ErrInvalid
	}
	// The decision lock also serializes identical assignment retries.
	previous, err := read(ctx, tx, p.ProjectID, decision)
	if err == nil {
		if tx.Commit(ctx) != nil {
			return Assignment{}, ErrUnavailable
		}
		return previous, nil
	}
	if err != pgx.ErrNoRows {
		return Assignment{}, err
	}
	var owner int64
	var key []byte
	var epoch int64
	err = tx.QueryRow(ctx, `SELECT owner_github_id,public_key,credential_epoch FROM mailbox.project_runners WHERE project_id=$1 AND id=$2 AND role='executor' AND active AND credential_hash IS NOT NULL AND credential_expires_at>clock_timestamp() FOR SHARE`, p.ProjectID, action.Target).Scan(&owner, &key, &epoch)
	if err != nil {
		return Assignment{}, ErrOffline
	}
	if s.Identities.Authority.Verify(ctx, p.Repository, owner, true) != nil {
		return Assignment{}, ErrForbidden
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, machine(owner, key)); err != nil {
		return Assignment{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO mailbox.executor_slots(owner_github_id,public_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, owner, key); err != nil {
		return Assignment{}, ErrUnavailable
	}
	var occupied *string
	var generation int64
	err = tx.QueryRow(ctx, `SELECT assignment_id::text,generation FROM mailbox.executor_slots WHERE owner_github_id=$1 AND public_key=$2 FOR UPDATE`, owner, key).Scan(&occupied, &generation)
	if err != nil {
		return Assignment{}, ErrUnavailable
	}
	if occupied != nil {
		return Assignment{}, ErrOccupied
	}
	var raw []byte
	var hash string
	err = tx.QueryRow(ctx, `SELECT client,client_hash FROM mailbox.executor_presence WHERE project_id=$1 AND runner_id=$2 AND credential_epoch=$3 AND available AND lease_until>clock_timestamp() FOR SHARE`, p.ProjectID, action.Target, epoch).Scan(&raw, &hash)
	if err != nil {
		return Assignment{}, ErrOffline
	}
	var client Client
	if json.Unmarshal(raw, &client) != nil {
		return Assignment{}, ErrUnavailable
	}
	canonical, actual, err := client.canonical()
	if err != nil || actual != hash {
		return Assignment{}, ErrUnavailable
	}
	generation++
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO mailbox.work_assignments(project_id,root_id,contract_id,decision_id,task_id,executor_id,owner_github_id,public_key,slot_generation,client,client_hash)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, p.ProjectID, record.Root, record.Contract, decision, task, action.Target, owner, key, generation, canonical, hash).Scan(&id)
	if err != nil {
		var failure *pgconn.PgError
		if errors.As(err, &failure) && failure.Code == "23505" && failure.ConstraintName == "work_assignments_active_task" {
			return Assignment{}, ErrTaskReserved
		}
		return Assignment{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `UPDATE mailbox.executor_slots SET generation=$3,assignment_id=$4 WHERE owner_github_id=$1 AND public_key=$2`, owner, key, generation, id)
	if err != nil {
		return Assignment{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.audit_events(workspace_id,actor_kind,action,target_id,metadata) SELECT workspace_id,'agent','work.reserved',$2,jsonb_build_object('project_id',$1::text,'decision_id',$3::text,'executor_id',$4::text,'generation',$5::bigint) FROM mailbox.orchestration_projects WHERE id=$1::uuid`, p.ProjectID, id, decision, action.Target, generation)
	if err != nil {
		return Assignment{}, ErrUnavailable
	}
	out, err := read(ctx, tx, p.ProjectID, decision)
	if err != nil {
		return Assignment{}, err
	}
	var change, base string
	if err = tx.QueryRow(ctx, `SELECT content->>'change',content->>'base_commit' FROM mailbox.work_contracts WHERE project_id=$1 AND id=$2`, p.ProjectID, record.Contract).Scan(&change, &base); err != nil {
		return Assignment{}, ErrUnavailable
	}
	_, err = controlevents.PublishTx(ctx, tx, p.ProjectID, action.Target, "assignment:"+id, &pb.ServerFrame{Body: &pb.ServerFrame_Assignment{Assignment: &pb.Assignment{AssignmentId: id, ContractHash: record.Material.SpecDigest, Repository: p.Repository.Owner + "/" + p.Repository.Name, Branch: "openspec/" + change, BaseCommit: base, OpenspecChange: change, Instruction: "Implement OpenSpec task " + task + " from " + change}}})
	if err != nil {
		return Assignment{}, err
	}
	if tx.Commit(ctx) != nil {
		return Assignment{}, ErrUnavailable
	}
	return out, nil
}

// Busy returns only an occupancy bit, never another project's task contents.
func (s *Store) Busy(ctx context.Context, token string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token, pb.Role_ROLE_EXECUTOR)
	if err != nil {
		return false, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return false, err
	}
	var busy bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.executor_slots WHERE owner_github_id=$1 AND public_key=$2 AND assignment_id IS NOT NULL)`, p.GitHubID, p.Key).Scan(&busy)
	if err != nil || tx.Commit(ctx) != nil {
		return false, ErrUnavailable
	}
	return busy, nil
}
