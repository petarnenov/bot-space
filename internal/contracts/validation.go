package contracts

import (
	"context"
	"time"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/security"
)

// Validator uses operator-configured repository/tool mappings. Model requests
// cannot provide a validation result, arbitrary checkout or command executable.
type Validator struct {
	Store    *Store
	Checkout func(project string, repositoryID int64) (string, error)
	OpenSpec string
}

func (v *Validator) Validate(ctx context.Context, token, project, id string) (string, error) {
	if v.Store == nil || v.Checkout == nil {
		return "", ErrInvalid
	}
	record, err := v.Store.Get(ctx, token, project, id)
	if err != nil {
		return "", err
	}
	_, digest, err := record.Content.Canonical()
	if err != nil || digest != record.Hash {
		return "", ErrInvalid
	}
	checkout, err := v.Checkout(project, record.Content.RepositoryID)
	if err != nil {
		return "", ErrArtifacts
	}
	if err = record.Content.VerifySpecification(ctx, checkout, v.OpenSpec); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	principal, err := v.Store.Identities.Authenticate(ctx, token)
	if err != nil || principal.ProjectID != project || principal.Role != pb.Role_ROLE_ARCHITECT {
		return "", ErrForbidden
	}
	tx, err := v.Store.Pool.Begin(ctx)
	if err != nil {
		return "", ErrUnavailable
	}
	defer rollback(tx)
	var current bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.work_contracts c JOIN mailbox.human_intentions i ON i.project_id=c.project_id AND i.id=c.root_id
 JOIN mailbox.orchestration_projects p ON p.id=c.project_id JOIN mailbox.project_runners r ON r.project_id=c.project_id
 WHERE c.project_id=$1 AND c.id=$2 AND c.content_hash=$3 AND i.current_revision=c.root_revision AND i.state NOT IN ('cancelled','completed') AND p.active
 AND r.id=$4 AND r.active AND r.role='architect' AND r.credential_hash=$5 AND r.credential_epoch=$6 AND r.credential_expires_at>clock_timestamp())`, project, id, digest, principal.RunnerID, security.Hash(token), principal.CredentialEpoch).Scan(&current)
	if err != nil {
		return "", ErrUnavailable
	}
	if !current {
		return "", ErrStale
	}
	var validation string
	err = tx.QueryRow(ctx, `INSERT INTO mailbox.contract_validations(project_id,contract_id,contract_hash,validator_runner_id) VALUES($1,$2,$3,$4)
 ON CONFLICT(project_id,contract_id,contract_hash) DO UPDATE SET contract_hash=EXCLUDED.contract_hash RETURNING id::text`, project, id, digest, principal.RunnerID).Scan(&validation)
	if err != nil {
		return "", ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.audit_events(workspace_id,actor_kind,action,target_id,metadata)
 SELECT p.workspace_id,'agent','contract.validated',$1,jsonb_build_object('project_id',$2::text,'contract_id',$3::text,'contract_hash',$4::text)
 FROM mailbox.orchestration_projects p WHERE p.id=$2 AND NOT EXISTS(SELECT 1 FROM mailbox.audit_events WHERE action='contract.validated' AND target_id=$1)`, validation, project, id, digest)
	if err != nil {
		return "", ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return "", ErrUnavailable
	}
	return validation, nil
}

// RequireValidated is a gate for the allocator; it grants no council approval.
func (s *Store) RequireValidated(ctx context.Context, project, id, digest string) error {
	if !security.ValidUUID(project) || !security.ValidUUID(id) || !hash.MatchString(digest) {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var valid bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.work_contracts c JOIN mailbox.contract_validations v
 ON v.project_id=c.project_id AND v.contract_id=c.id AND v.contract_hash=c.content_hash
 JOIN mailbox.human_intentions i ON i.project_id=c.project_id AND i.id=c.root_id JOIN mailbox.orchestration_projects p ON p.id=c.project_id
 WHERE c.project_id=$1 AND c.id=$2 AND c.content_hash=$3 AND i.current_revision=c.root_revision AND i.state NOT IN ('cancelled','completed') AND p.active)`, project, id, digest).Scan(&valid)
	if err != nil {
		return ErrUnavailable
	}
	if !valid {
		return ErrStale
	}
	return nil
}
