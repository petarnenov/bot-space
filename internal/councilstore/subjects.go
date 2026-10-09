package councilstore

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/contracts"
	"github.com/petarnenov/bot-space/internal/council"
	"github.com/petarnenov/bot-space/internal/security"
)

// OpenAllocation deliberates one OpenSpec task and executor. Acceptance alone
// does not reserve capacity; allocation must consume it under the global slot
// and current-source transaction fences.
func (s *Store) OpenAllocation(ctx context.Context, token, contract, task string, proposal council.Proposal) (Record, error) {
	if len(task) < 1 || len(task) > 32 {
		return Record{}, council.ErrInvalid
	}
	return s.openDecision(ctx, token, contract, "allocation", "task:"+task, proposal, false)
}
func (s *Store) ReconsiderAllocation(ctx context.Context, token, contract, task string, proposal council.Proposal) (Record, error) {
	if len(task) < 1 || len(task) > 32 {
		return Record{}, council.ErrInvalid
	}
	return s.openDecision(ctx, token, contract, "allocation", "task:"+task, proposal, true)
}

func (s *Store) proposal(ctx context.Context, tx pgx.Tx, p authenticated, contract, kind, subject string, proposal council.Proposal) error {
	if kind == "answer" {
		if !answerProposal(proposal, subject) {
			return council.ErrInvalid
		}
		_, err := questionSource(ctx, tx, p.ProjectID, contract, subject)
		return err
	}
	if kind == "plan" {
		if subject != "root" || !planProposal(proposal, contract) {
			return council.ErrInvalid
		}
		return nil
	}
	if kind != "allocation" || !strings.HasPrefix(subject, "task:") || len(proposal.Actions) != 1 {
		return council.ErrInvalid
	}
	task := strings.TrimPrefix(subject, "task:")
	action := proposal.Actions[0]
	if action.Kind != "assign" || action.Value != task || !security.ValidUUID(action.Target) {
		return council.ErrInvalid
	}
	var raw []byte
	var digest string
	if err := tx.QueryRow(ctx, `SELECT content,content_hash FROM mailbox.work_contracts WHERE project_id=$1 AND id=$2 FOR SHARE`, p.ProjectID, contract).Scan(&raw, &digest); err != nil {
		return ErrStale
	}
	var content contracts.Content
	if json.Unmarshal(raw, &content) != nil {
		return ErrUnavailable
	}
	canonical, computed, err := content.Canonical()
	if err != nil || computed != digest || canonical.ProjectID != p.ProjectID {
		return ErrUnavailable
	}
	found := false
	for _, id := range canonical.Tasks {
		if id == task {
			found = true
			break
		}
	}
	if !found {
		return council.ErrInvalid
	}
	var github int64
	err = tx.QueryRow(ctx, `SELECT owner_github_id FROM mailbox.project_runners WHERE project_id=$1 AND id=$2 AND role='executor' AND active FOR SHARE`, p.ProjectID, action.Target).Scan(&github)
	if err != nil {
		return ErrForbidden
	}
	if s.Identities.Authority.Verify(ctx, p.Repository, github, true) != nil {
		return ErrForbidden
	}
	return nil
}

// Current-head checking prevents an older decision from collecting votes after
// a linked replacement. Historical Get remains available without this gate.
func current(ctx context.Context, tx pgx.Tx, r Record) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT decision_id::text FROM mailbox.council_heads WHERE project_id=$1 AND root_id=$2 AND kind=$3 AND subject=$4 AND decision_id=$5 FOR SHARE`, r.Project, r.Root, r.Snapshot.Kind, r.Subject, r.Snapshot.ID).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrStale
		}
		return ErrUnavailable
	}
	return nil
}

// LockAccepted is a transaction gate for trusted control-service consumers.
// It checks exact kind, subject and contract hash, current head/source fences
// and actual majority evidence. Callers must also validate their own actor and
// operational capability and hold assignment/slot locks in this transaction.
func LockAccepted(ctx context.Context, tx pgx.Tx, project, id, kind, subject, digest string) (Record, error) {
	if !security.ValidUUID(project) || !security.ValidUUID(id) || len(subject) < 1 || len(subject) > 256 {
		return Record{}, council.ErrInvalid
	}
	r, _, err := load(ctx, tx, project, id)
	if err != nil {
		return Record{}, err
	}
	if r.Snapshot.Status != council.Accepted || r.Snapshot.Kind != kind || r.Subject != subject || r.Material.SpecDigest != digest {
		return Record{}, ErrStale
	}
	if err = current(ctx, tx, r); err != nil {
		return Record{}, err
	}
	_, material, err := source(ctx, tx, project, r.Contract)
	if err != nil || material.RootRevision != r.Material.RootRevision || material.SpecDigest != r.Material.SpecDigest {
		return Record{}, ErrStale
	}
	if kind == "answer" {
		fingerprint, e := questionSource(ctx, tx, project, r.Contract, subject)
		if e != nil {
			return Record{}, e
		}
		if fingerprint != r.Material.EvidenceDigest {
			return Record{}, ErrStale
		}
	}
	if kind == "allocation" {
		var planID string
		err = tx.QueryRow(ctx, `SELECT decision_id::text FROM mailbox.council_heads WHERE project_id=$1 AND root_id=$2 AND kind='plan' AND subject='root'`, project, r.Root).Scan(&planID)
		if err != nil {
			return Record{}, ErrStale
		}
		plan, _, err := load(ctx, tx, project, planID)
		if err != nil {
			return Record{}, err
		}
		if plan.Snapshot.Status != council.Accepted || plan.Contract != r.Contract || plan.Material.SpecDigest != r.Material.SpecDigest || plan.Material.RootRevision != r.Material.RootRevision {
			return Record{}, ErrStale
		}
		if err = current(ctx, tx, plan); err != nil {
			return Record{}, err
		}
	}
	return r, nil
}
