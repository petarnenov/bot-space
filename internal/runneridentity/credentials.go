package runneridentity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/control"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/security"
)

const TokenPrefix = "bsr_"
const CredentialLifetime = 15 * time.Minute

type Challenge struct {
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Credential struct {
	RunnerID  string    `json:"runner_id"`
	ProjectID string    `json:"project_id"`
	Role      Role      `json:"role"`
	Epoch     uint64    `json:"epoch"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Challenge creates a short-lived nonce. HTTP routing must bound admissions;
// obtaining a nonce does not authorize any operation or reveal a credential.
func (s *Store) Challenge(ctx context.Context, id, purpose string) (Challenge, error) {
	if !security.ValidUUID(id) || (purpose != "claim" && purpose != "refresh") {
		return Challenge{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	nonce, err := security.Secret()
	if err != nil {
		return Challenge{}, ErrUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Challenge{}, ErrUnavailable
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); err != nil {
		return Challenge{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `DELETE FROM mailbox.runner_key_challenges WHERE expires_at<=clock_timestamp() AND (enrollment_id=$1 OR runner_id=$1)`, id); err != nil {
		return Challenge{}, ErrUnavailable
	}
	var outstanding int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM mailbox.runner_key_challenges WHERE enrollment_id=$1 OR runner_id=$1`, id).Scan(&outstanding); err != nil {
		return Challenge{}, ErrUnavailable
	}
	if outstanding >= 16 {
		return Challenge{}, ErrUnavailable
	}
	var out Challenge
	out.Nonce = nonce
	var query string
	if purpose == "claim" {
		query = `INSERT INTO mailbox.runner_key_challenges(nonce_hash,enrollment_id,purpose)
 SELECT $2,e.id,'claim' FROM mailbox.runner_enrollments e JOIN mailbox.orchestration_projects p ON p.id=e.project_id
 WHERE e.id=$1 AND e.expires_at>clock_timestamp() AND e.authorized_github_id IS NOT NULL AND p.active RETURNING expires_at`
	} else {
		query = `INSERT INTO mailbox.runner_key_challenges(nonce_hash,runner_id,purpose)
 SELECT $2,r.id,'refresh' FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id
 WHERE r.id=$1 AND r.active AND p.active RETURNING expires_at`
	}
	if err = tx.QueryRow(ctx, query, id, security.Hash(nonce)).Scan(&out.ExpiresAt); err != nil {
		return Challenge{}, ErrUnauthenticated
	}
	if tx.Commit(ctx) != nil {
		return Challenge{}, ErrUnavailable
	}
	return out, nil
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// Issue verifies the key and current GitHub authority, consumes the challenge,
// rotates the credential and writes safe audit evidence in one transaction.
func (s *Store) Issue(ctx context.Context, id, purpose, nonce string, signature []byte) (Credential, error) {
	if _, err := ChallengeMessage(id, purpose, nonce); err != nil {
		return Credential{}, err
	}
	if s.Authority == nil {
		return Credential{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Credential{}, ErrUnavailable
	}
	defer rollback(tx)
	var public []byte
	var githubID int64
	var project, workspace string
	var role Role
	var repo repositoryaccess.Repository
	// Serialize per resource before checking challenge eligibility. This also
	// prevents two concurrent copies of one proof issuing separate credentials.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); err != nil {
		return Credential{}, ErrUnavailable
	}
	if purpose == "claim" {
		err = tx.QueryRow(ctx, `SELECT e.public_key,e.authorized_github_id,e.project_id::text,p.workspace_id::text,e.role,
 p.repository_id,p.repository_owner_id,p.repository_owner,p.repository_name FROM mailbox.runner_enrollments e
 JOIN mailbox.orchestration_projects p ON p.id=e.project_id WHERE e.id=$1 AND p.active
 AND e.authorized_github_id IS NOT NULL AND e.expires_at>clock_timestamp() FOR SHARE OF e,p`, id).Scan(&public, &githubID, &project, &workspace, &role, &repo.ID, &repo.OwnerID, &repo.Owner, &repo.Name)
	} else {
		err = tx.QueryRow(ctx, `SELECT r.public_key,r.owner_github_id,r.project_id::text,p.workspace_id::text,r.role,
 p.repository_id,p.repository_owner_id,p.repository_owner,p.repository_name FROM mailbox.project_runners r
 JOIN mailbox.orchestration_projects p ON p.id=r.project_id WHERE r.id=$1 AND r.active AND p.active FOR UPDATE OF r FOR SHARE OF p`, id).Scan(&public, &githubID, &project, &workspace, &role, &repo.ID, &repo.OwnerID, &repo.Owner, &repo.Name)
	}
	if err != nil {
		return Credential{}, ErrUnauthenticated
	}
	if err = VerifyChallenge(public, id, purpose, nonce, signature); err != nil {
		return Credential{}, err
	}
	var challengeExists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.runner_key_challenges WHERE nonce_hash=$1 AND purpose=$2
 AND expires_at>clock_timestamp() AND (($2='claim' AND enrollment_id=$3) OR ($2='refresh' AND runner_id=$3)))`, security.Hash(nonce), purpose, id).Scan(&challengeExists)
	if err != nil {
		return Credential{}, ErrUnavailable
	}
	if !challengeExists {
		return Credential{}, ErrUnauthenticated
	}
	if err = s.Authority.Verify(ctx, repo, githubID, true); err != nil {
		if errors.Is(err, repositoryaccess.ErrDenied) {
			return Credential{}, ErrUnauthenticated
		}
		return Credential{}, ErrUnavailable
	}
	// A physical role keeps one GitHub actor across repository scopes.
	// Serialize claims by key/role after network verification, then reject
	// attempts to bind this key to another person on another project.
	machineScope := security.Hash(string(public)) + ":" + string(role)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", machineScope); err != nil {
		return Credential{}, ErrUnavailable
	}
	var conflictingActor bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.project_runners WHERE public_key=$1 AND role=$2 AND owner_github_id<>$3)`, public, role, githubID).Scan(&conflictingActor); err != nil {
		return Credential{}, ErrUnavailable
	}
	if conflictingActor {
		return Credential{}, ErrUnauthenticated
	}
	secret, err := security.Secret()
	if err != nil {
		return Credential{}, ErrUnavailable
	}
	out := Credential{ProjectID: project, Role: role, Token: TokenPrefix + secret}
	// Recheck expiring source/challenge at the write, not only before GitHub I/O.
	if purpose == "claim" {
		err = tx.QueryRow(ctx, `INSERT INTO mailbox.project_runners(project_id,owner_github_id,public_key,role,credential_hash,credential_expires_at)
 SELECT project_id,authorized_github_id,public_key,role,$2,clock_timestamp()+interval '15 minutes'
 FROM mailbox.runner_enrollments WHERE id=$1 AND expires_at>clock_timestamp()
 ON CONFLICT(project_id,public_key,role) DO UPDATE SET credential_epoch=mailbox.project_runners.credential_epoch+1,
 credential_hash=EXCLUDED.credential_hash,credential_expires_at=EXCLUDED.credential_expires_at
 WHERE mailbox.project_runners.active AND mailbox.project_runners.owner_github_id=EXCLUDED.owner_github_id
 RETURNING id::text,credential_epoch,credential_expires_at`, id, security.Hash(out.Token)).Scan(&out.RunnerID, &out.Epoch, &out.ExpiresAt)
	} else {
		err = tx.QueryRow(ctx, `UPDATE mailbox.project_runners SET credential_epoch=credential_epoch+1,
 credential_hash=$2,credential_expires_at=clock_timestamp()+interval '15 minutes' WHERE id=$1 AND active
 RETURNING id::text,credential_epoch,credential_expires_at`, id, security.Hash(out.Token)).Scan(&out.RunnerID, &out.Epoch, &out.ExpiresAt)
	}
	if err != nil {
		return Credential{}, ErrUnauthenticated
	}
	deleted, err := tx.Exec(ctx, `DELETE FROM mailbox.runner_key_challenges WHERE nonce_hash=$1 AND expires_at>clock_timestamp()`, security.Hash(nonce))
	if err != nil {
		return Credential{}, ErrUnavailable
	}
	if deleted.RowsAffected() != 1 {
		return Credential{}, ErrUnauthenticated
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.audit_events(workspace_id,actor_kind,action,target_id,metadata)
 VALUES($1,'system','runner.credential_issued',$2,jsonb_build_object('project_id',$3::text,'role',$4::text,'epoch',$5::bigint,'github_id',$6::bigint))`, workspace, out.RunnerID, project, string(role), out.Epoch, githubID)
	if err != nil || tx.Commit(ctx) != nil {
		return Credential{}, ErrUnavailable
	}
	return out, nil
}

// Authenticate is suitable for control.Authenticate: each call checks current
// active state/epoch and current repository authority, including stream frames.
func (s *Store) Authenticate(ctx context.Context, token string) (control.Principal, error) {
	if len(token) != len(TokenPrefix)+43 || token[:len(TokenPrefix)] != TokenPrefix || s.Authority == nil {
		return control.Principal{}, ErrUnauthenticated
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var principal control.Principal
	var role Role
	var githubID int64
	var repo repositoryaccess.Repository
	hash := security.Hash(token)
	err := s.Pool.QueryRow(ctx, `SELECT r.id::text,r.project_id::text,r.role,r.credential_epoch,r.owner_github_id,
 p.repository_id,p.repository_owner_id,p.repository_owner,p.repository_name FROM mailbox.project_runners r
 JOIN mailbox.orchestration_projects p ON p.id=r.project_id WHERE r.credential_hash=$1 AND r.active AND p.active
 AND r.credential_expires_at>clock_timestamp()`, hash).Scan(&principal.RunnerID, &principal.ProjectID, &role, &principal.CredentialEpoch, &githubID, &repo.ID, &repo.OwnerID, &repo.Owner, &repo.Name)
	if err != nil {
		return control.Principal{}, ErrUnauthenticated
	}
	if err = s.Authority.Verify(ctx, repo, githubID, false); err != nil {
		return control.Principal{}, ErrUnauthenticated
	}
	var current bool
	err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id
 WHERE r.id=$1 AND r.credential_hash=$2 AND r.credential_epoch=$3 AND r.active AND p.active
 AND r.credential_expires_at>clock_timestamp() AND p.repository_id=$4 AND p.repository_owner_id=$5
 AND p.repository_owner=$6 AND p.repository_name=$7 AND r.role=$8 AND r.owner_github_id=$9)`, principal.RunnerID, hash, principal.CredentialEpoch, repo.ID, repo.OwnerID, repo.Owner, repo.Name, role, githubID).Scan(&current)
	if err != nil || !current {
		return control.Principal{}, ErrUnauthenticated
	}
	if role == Architect {
		principal.Role = pb.Role_ROLE_ARCHITECT
	} else if role == Executor {
		principal.Role = pb.Role_ROLE_EXECUTOR
	} else {
		return control.Principal{}, ErrUnauthenticated
	}
	return principal, nil
}
