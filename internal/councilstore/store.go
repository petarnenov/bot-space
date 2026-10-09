// Package councilstore commits authenticated council evidence in PostgreSQL.
package councilstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/control"
	"github.com/petarnenov/bot-space/internal/council"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
)

var ErrForbidden = errors.New("current architect authority required")
var ErrUnavailable = errors.New("council storage unavailable")
var ErrStale = errors.New("council source or coordinator is stale")

type Store struct {
	Pool       *pgxpool.Pool
	Identities *runneridentity.Store
}
type Record struct {
	Project, Root, Contract string
	Snapshot                council.Snapshot
	Material                council.Material
}
type Lease struct {
	Epoch     int64
	ExpiresAt time.Time
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) authenticate(ctx context.Context, token string) (control.Principal, error) {
	if s.Pool == nil || s.Identities == nil {
		return control.Principal{}, ErrUnavailable
	}
	p, err := s.Identities.Authenticate(ctx, token)
	if err != nil || p.Role != pb.Role_ROLE_ARCHITECT {
		return control.Principal{}, ErrForbidden
	}
	return p, nil
}
func actor(ctx context.Context, tx pgx.Tx, p control.Principal, token string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT r.id::text FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id
 WHERE r.id=$1 AND r.project_id=$2 AND r.active AND p.active AND r.role='architect' AND r.credential_epoch=$3 AND r.credential_hash=$4 AND r.credential_expires_at>clock_timestamp() FOR SHARE OF r,p`, p.RunnerID, p.ProjectID, p.CredentialEpoch, security.Hash(token)).Scan(&id)
	if err != nil {
		return ErrForbidden
	}
	return nil
}

func load(ctx context.Context, tx pgx.Tx, project, id string) (Record, *council.Decision, error) {
	var r Record
	var snap, material []byte
	err := tx.QueryRow(ctx, `SELECT project_id::text,root_id::text,contract_id::text,snapshot,material FROM mailbox.council_decisions WHERE project_id=$1 AND id=$2 FOR UPDATE`, project, id).Scan(&r.Project, &r.Root, &r.Contract, &snap, &material)
	if err == pgx.ErrNoRows {
		return r, nil, ErrForbidden
	}
	if err != nil {
		return r, nil, ErrUnavailable
	}
	if json.Unmarshal(snap, &r.Snapshot) != nil || json.Unmarshal(material, &r.Material) != nil {
		return r, nil, ErrUnavailable
	}
	if r.Snapshot.ID != id {
		return r, nil, ErrUnavailable
	}
	d, err := council.Restore(r.Snapshot, r.Material)
	if err != nil {
		return r, nil, ErrUnavailable
	}
	if err = checkEvidence(ctx, tx, r); err != nil {
		return r, nil, err
	}
	return r, d, nil
}

// source obtains a shared execution fence and immutable verified contract.
func source(ctx context.Context, tx pgx.Tx, project, contract string) (string, council.Material, error) {
	var root, digest string
	var revision int
	var epoch int64
	err := tx.QueryRow(ctx, `SELECT c.root_id::text,c.root_revision,i.lifecycle_epoch,c.content_hash
 FROM mailbox.work_contracts c JOIN mailbox.human_intentions i ON i.project_id=c.project_id AND i.id=c.root_id
 JOIN mailbox.contract_validations v ON v.project_id=c.project_id AND v.contract_id=c.id AND v.contract_hash=c.content_hash
 WHERE c.project_id=$1 AND c.id=$2`, project, contract).Scan(&root, &revision, &epoch, &digest)
	if err != nil {
		return "", council.Material{}, ErrStale
	}
	if backlog.LockExecutable(ctx, tx, project, root, revision, epoch) != nil {
		return "", council.Material{}, ErrStale
	}
	return root, council.Material{RootRevision: fmt.Sprintf("%s:%d:%d", root, revision, epoch), SpecDigest: digest}, nil
}

func persist(ctx context.Context, tx pgx.Tx, r Record, d *council.Decision) error {
	snapshot := d.Snapshot()
	raw, _ := json.Marshal(snapshot)
	if _, err := tx.Exec(ctx, `UPDATE mailbox.council_decisions SET snapshot=$3 WHERE project_id=$1 AND id=$2`, r.Project, snapshot.ID, raw); err != nil {
		return ErrUnavailable
	}
	for _, round := range snapshot.Rounds {
		proposal, _ := json.Marshal(round.Proposal)
		if _, err := tx.Exec(ctx, `INSERT INTO mailbox.council_rounds(project_id,decision_id,round,proposal_hash,proposal) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, r.Project, snapshot.ID, round.Number, round.Hash, proposal); err != nil {
			return ErrUnavailable
		}
		for _, vote := range round.Votes {
			if _, err := tx.Exec(ctx, `INSERT INTO mailbox.council_votes(project_id,decision_id,round,runner_id,choice) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, r.Project, snapshot.ID, round.Number, vote.Member, vote.Choice); err != nil {
				return ErrUnavailable
			}
		}
	}
	if snapshot.Status == council.Accepted {
		round := snapshot.Rounds[len(snapshot.Rounds)-1]
		inserted, err := tx.Exec(ctx, `INSERT INTO mailbox.council_commits(project_id,decision_id,round,proposal_hash) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, r.Project, snapshot.ID, round.Number, round.Hash)
		if err != nil {
			return ErrUnavailable
		}
		if inserted.RowsAffected() == 1 {
			_, err = tx.Exec(ctx, `INSERT INTO mailbox.audit_events(workspace_id,actor_kind,action,target_id,metadata) SELECT workspace_id,'agent','council.accepted',$2,jsonb_build_object('project_id',$1::text,'proposal_hash',$3::text,'round',$4::integer) FROM mailbox.orchestration_projects WHERE id=$1::uuid`, r.Project, snapshot.ID, round.Hash, round.Number)
			if err != nil {
				return ErrUnavailable
			}
		}
	}
	return nil
}

// OpenPlan uses one durable head per root. A new label cannot reset exhaustion.
// Other operational kinds will bind their authoritative work/question records
// through the same transaction machinery when those records are introduced.
func (s *Store) OpenPlan(ctx context.Context, token, contract string, proposal council.Proposal) (Record, error) {
	return s.openPlan(ctx, token, contract, proposal, false)
}
func (s *Store) ReconsiderPlan(ctx context.Context, token, contract string, proposal council.Proposal) (Record, error) {
	return s.openPlan(ctx, token, contract, proposal, true)
}
func (s *Store) openPlan(ctx context.Context, token, contract string, proposal council.Proposal, reconsider bool) (Record, error) {
	if !security.ValidUUID(contract) {
		return Record{}, council.ErrInvalid
	}
	if !planProposal(proposal, contract) {
		return Record{}, council.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	defer rollback(tx)
	root, material, err := source(ctx, tx, p.ProjectID, contract)
	if err != nil {
		return Record{}, err
	}
	if err = actor(ctx, tx, p, token); err != nil {
		return Record{}, err
	}
	// The lock is scoped by project/root/kind, before reading or creating a head.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, p.ProjectID+":"+root+":plan"); err != nil {
		return Record{}, ErrUnavailable
	}
	rows, err := tx.Query(ctx, `SELECT r.id::text,r.owner_github_id,p.repository_id,p.repository_owner_id,p.repository_owner,p.repository_name FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id WHERE r.project_id=$1 AND r.role='architect' AND r.active ORDER BY r.id FOR SHARE OF r,p`, p.ProjectID)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	members := []string{}
	type memberAuthority struct {
		actor int64
		repo  repositoryaccess.Repository
	}
	authorities := []memberAuthority{}
	for rows.Next() {
		var id string
		var authority memberAuthority
		if err = rows.Scan(&id, &authority.actor, &authority.repo.ID, &authority.repo.OwnerID, &authority.repo.Owner, &authority.repo.Name); err != nil {
			rows.Close()
			return Record{}, ErrUnavailable
		}
		members = append(members, id)
		authorities = append(authorities, authority)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Record{}, ErrUnavailable
	}
	for _, authority := range authorities {
		if s.Identities.Authority.Verify(ctx, authority.repo, authority.actor, true) != nil {
			return Record{}, ErrForbidden
		}
	}
	if !slices.Contains(members, p.RunnerID) {
		return Record{}, ErrForbidden
	}
	memberJSON, _ := json.Marshal(members)
	material.CouncilRevision = security.Hash(string(memberJSON))
	var head string
	err = tx.QueryRow(ctx, `SELECT decision_id::text FROM mailbox.council_heads WHERE project_id=$1 AND root_id=$2 AND kind='plan'`, p.ProjectID, root).Scan(&head)
	if err != nil && err != pgx.ErrNoRows {
		return Record{}, ErrUnavailable
	}
	var previous *council.Decision
	if head != "" {
		old, d, e := load(ctx, tx, p.ProjectID, head)
		if e != nil {
			return Record{}, e
		}
		if !reconsider {
			if old.Contract != contract {
				return Record{}, ErrStale
			}
			raw, _ := json.Marshal(proposal)
			saved, _ := json.Marshal(old.Snapshot.Rounds[0].Proposal)
			if string(raw) != string(saved) {
				return Record{}, council.ErrState
			}
			if tx.Commit(ctx) != nil {
				return Record{}, ErrUnavailable
			}
			return old, nil
		}
		previous = d
	} else if reconsider {
		return Record{}, council.ErrState
	}
	var id string
	if err = tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		return Record{}, ErrUnavailable
	}
	var d *council.Decision
	if previous == nil {
		d, err = council.New(id, "plan", members, material, proposal)
	} else if previous.Snapshot().Status != council.Blocked &&
		(material.RootRevision != previous.Material().RootRevision || material.SpecDigest != previous.Material().SpecDigest) {
		// New authoritative input invalidates an earlier in-flight or accepted
		// plan. Preserve that history and link its replacement; caller labels
		// and membership changes alone cannot take this path.
		d, err = council.New(id, "plan", members, material, proposal)
		if err == nil {
			snapshot := d.Snapshot()
			snapshot.PreviousID = previous.Snapshot().ID
			d, err = council.Restore(snapshot, material)
		}
	} else {
		d, err = previous.Reconsider(id, members, material, proposal)
	}
	if err != nil {
		return Record{}, err
	}
	r := Record{Project: p.ProjectID, Root: root, Contract: contract, Material: material, Snapshot: d.Snapshot()}
	raw, _ := json.Marshal(r.Snapshot)
	mat, _ := json.Marshal(material)
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.council_decisions(id,project_id,root_id,contract_id,kind,previous_id,material,snapshot) VALUES($1,$2,$3,$4,'plan',NULLIF($5,'')::uuid,$6,$7)`, id, p.ProjectID, root, contract, r.Snapshot.PreviousID, mat, raw)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.council_heads(project_id,root_id,kind,decision_id) VALUES($1,$2,'plan',$3) ON CONFLICT(project_id,root_id,kind) DO UPDATE SET decision_id=EXCLUDED.decision_id`, p.ProjectID, root, id)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	if err = persist(ctx, tx, r, d); err != nil {
		return Record{}, err
	}
	if tx.Commit(ctx) != nil {
		return Record{}, ErrUnavailable
	}
	return r, nil
}

// Get returns evidence, including historical/fenced decisions. It does not
// grant execution authority; consuming actions must recheck current sources.
func (s *Store) Get(ctx context.Context, token, id string) (Record, error) {
	if !security.ValidUUID(id) {
		return Record{}, council.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Record{}, err
	}
	r, _, err := load(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Record{}, err
	}
	if tx.Commit(ctx) != nil {
		return Record{}, ErrUnavailable
	}
	return r, nil
}

func planProposal(proposal council.Proposal, contract string) bool {
	if len(proposal.Actions) == 0 {
		return false
	}
	for _, action := range proposal.Actions {
		if action.Kind != "plan" || action.Target != contract {
			return false
		}
	}
	return true
}

func (s *Store) Vote(ctx context.Context, token, id string, round int, hash string, choice council.Choice) (Record, error) {
	if !security.ValidUUID(id) {
		return Record{}, council.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Record{}, err
	}
	r, d, err := load(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Record{}, err
	}
	_, material, err := source(ctx, tx, p.ProjectID, r.Contract)
	if err != nil || material.RootRevision != r.Material.RootRevision || material.SpecDigest != r.Material.SpecDigest {
		return Record{}, ErrStale
	}
	if err = d.Cast(p.RunnerID, round, hash, choice); err != nil {
		return Record{}, err
	}
	if err = persist(ctx, tx, r, d); err != nil {
		return Record{}, err
	}
	r.Snapshot = d.Snapshot()
	if tx.Commit(ctx) != nil {
		return Record{}, ErrUnavailable
	}
	return r, nil
}
