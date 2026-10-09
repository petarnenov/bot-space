package runneridentity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/security"
)

type Authority interface {
	Verify(context.Context, repositoryaccess.Repository, int64, bool) error
}
type Store struct {
	Pool      *pgxpool.Pool
	Authority Authority
}
type Enrollment struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Role      Role      `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Begin requires proof of the requesting key, but never grants a credential.
// Repeating the same nonce returns the same unexpired enrollment. The browser
// OAuth callback must separately establish and verify the GitHub owner.
func (s *Store) Begin(ctx context.Context, p StartProof) (Enrollment, error) {
	if err := VerifyStart(p); err != nil {
		return Enrollment{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var e Enrollment
	err := s.Pool.QueryRow(ctx, `INSERT INTO mailbox.runner_enrollments(project_id,public_key,role,client_nonce_hash)
 SELECT id,$2,$3,$4 FROM mailbox.orchestration_projects WHERE id=$1 AND active
 ON CONFLICT(project_id,public_key,role,client_nonce_hash) DO UPDATE SET client_nonce_hash=EXCLUDED.client_nonce_hash
 WHERE mailbox.runner_enrollments.expires_at>clock_timestamp()
 RETURNING id::text,project_id::text,role,expires_at`, p.ProjectID, p.PublicKey, p.Role, security.Hash(p.Nonce)).Scan(&e.ID, &e.ProjectID, &e.Role, &e.ExpiresAt)
	if err != nil {
		return Enrollment{}, ErrUnavailable
	}
	return e, nil
}

// AuthorizeGitHub is only for a verified OAuth callback. Browser session reads
// and ordinary GET routes must never invoke it as an enrollment approval.
func (s *Store) AuthorizeGitHub(ctx context.Context, id string, githubID int64) error {
	if !security.ValidUUID(id) || githubID < 1 || s.Authority == nil {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var repo repositoryaccess.Repository
	err := s.Pool.QueryRow(ctx, `SELECT p.repository_id,p.repository_owner_id,p.repository_owner,p.repository_name
 FROM mailbox.runner_enrollments e JOIN mailbox.orchestration_projects p ON p.id=e.project_id
 WHERE e.id=$1 AND e.expires_at>clock_timestamp() AND p.active`, id).Scan(&repo.ID, &repo.OwnerID, &repo.Owner, &repo.Name)
	if err != nil {
		return ErrUnavailable
	}
	if err = s.Authority.Verify(ctx, repo, githubID, true); err != nil {
		if errors.Is(err, repositoryaccess.ErrDenied) {
			return ErrUnauthenticated
		}
		return ErrUnavailable
	}
	// An already-authorized attempt cannot be rebound to a different person.
	result, err := s.Pool.Exec(ctx, `UPDATE mailbox.runner_enrollments e SET authorized_github_id=$2,authorized_at=clock_timestamp()
 FROM mailbox.orchestration_projects p WHERE e.id=$1 AND p.id=e.project_id AND p.active
 AND e.expires_at>clock_timestamp() AND (e.authorized_github_id IS NULL OR e.authorized_github_id=$2)
    AND p.repository_id=$3 AND p.repository_owner_id=$4 AND p.repository_owner=$5 AND p.repository_name=$6`, id, githubID, repo.ID, repo.OwnerID, repo.Owner, repo.Name)
	if err != nil {
		return ErrUnavailable
	}
	if result.RowsAffected() != 1 {
		return ErrUnauthenticated
	}
	return nil
}
