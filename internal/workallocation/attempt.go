package workallocation

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/councilstore"
	"github.com/petarnenov/bot-space/internal/security"
)

var ErrAuthorityLost = errors.New("execution authority is not current")

type Attempt struct {
	ID, Assignment, Session, State string
	Number                         int
	AuthorityEpoch                 int64
	AuthorityUntil                 time.Time
}

func attempt(ctx context.Context, tx pgx.Tx, project, assignment string) (Attempt, error) {
	var a Attempt
	err := tx.QueryRow(ctx, `SELECT id::text,assignment_id::text,COALESCE(session_id,''),state,number,authority_epoch,authority_until FROM mailbox.work_attempts WHERE project_id=$1 AND assignment_id=$2 ORDER BY number DESC LIMIT 1 FOR UPDATE`, project, assignment).Scan(&a.ID, &a.Assignment, &a.Session, &a.State, &a.Number, &a.AuthorityEpoch, &a.AuthorityUntil)
	if err == pgx.ErrNoRows {
		return a, err
	}
	if err != nil {
		return a, ErrUnavailable
	}
	return a, nil
}

// execution checks current majority/root authority before assignment/slot locks.
func execution(ctx context.Context, tx pgx.Tx, p authenticated, id string) (string, error) {
	var decision, hash string
	err := tx.QueryRow(ctx, `SELECT a.decision_id::text,c.content_hash FROM mailbox.work_assignments a JOIN mailbox.work_contracts c ON c.project_id=a.project_id AND c.id=a.contract_id WHERE a.project_id=$1 AND a.id=$2 AND a.executor_id=$3 AND a.owner_github_id=$4 AND a.public_key=$5`, p.ProjectID, id, p.RunnerID, p.GitHubID, p.Key).Scan(&decision, &hash)
	if err != nil {
		return "", ErrForbidden
	}
	var subject string
	if err = tx.QueryRow(ctx, `SELECT 'task:'||task_id FROM mailbox.work_assignments WHERE project_id=$1 AND id=$2`, p.ProjectID, id).Scan(&subject); err != nil {
		return "", ErrUnavailable
	}
	if _, err = councilstore.LockAccepted(ctx, tx, p.ProjectID, decision, "allocation", subject, hash); err != nil {
		return "", err
	}
	var clientHash string
	err = tx.QueryRow(ctx, `SELECT a.client_hash FROM mailbox.work_assignments a JOIN mailbox.executor_slots s ON s.assignment_id=a.id AND s.owner_github_id=a.owner_github_id AND s.public_key=a.public_key AND s.generation=a.slot_generation
 JOIN mailbox.executor_presence e ON e.project_id=a.project_id AND e.runner_id=a.executor_id AND e.credential_epoch=$4 AND e.client_hash=a.client_hash AND e.lease_until>clock_timestamp()
 WHERE a.project_id=$1 AND a.id=$2 AND a.executor_id=$3 AND a.state IN ('reserved','running','question_wait') FOR UPDATE OF a FOR SHARE OF s,e`, p.ProjectID, id, p.RunnerID, p.CredentialEpoch).Scan(&clientHash)
	if err != nil {
		return "", ErrAuthorityLost
	}
	return clientHash, nil
}

// Begin returns the same first attempt on retries. It never creates a fresh
// context to replace interrupted/uncertain work; retries require council review.
func (s *Store) Begin(ctx context.Context, token, id string, client Client) (Attempt, error) {
	if !security.ValidUUID(id) {
		return Attempt{}, ErrInvalid
	}
	_, hash, err := client.canonical()
	if err != nil {
		return Attempt{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token, pb.Role_ROLE_EXECUTOR)
	if err != nil {
		return Attempt{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Attempt{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Attempt{}, err
	}
	expected, err := execution(ctx, tx, p, id)
	if err != nil {
		return Attempt{}, err
	}
	if expected != hash {
		return Attempt{}, ErrInvalid
	}
	previous, err := attempt(ctx, tx, p.ProjectID, id)
	if err == nil {
		var live bool
		if tx.QueryRow(ctx, `SELECT authority_until>clock_timestamp() FROM mailbox.work_attempts WHERE id=$1`, previous.ID).Scan(&live) != nil {
			return Attempt{}, ErrUnavailable
		}
		if !live || (previous.State != "starting" && previous.State != "running" && previous.State != "question_wait") {
			return Attempt{}, ErrAuthorityLost
		}
		if tx.Commit(ctx) != nil {
			return Attempt{}, ErrUnavailable
		}
		return previous, nil
	}
	if err != pgx.ErrNoRows {
		return Attempt{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.work_attempts(project_id,assignment_id,number,authority_until) VALUES($1,$2,1,clock_timestamp()+interval '90 seconds')`, p.ProjectID, id)
	if err != nil {
		return Attempt{}, ErrUnavailable
	}
	out, err := attempt(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Attempt{}, err
	}
	if tx.Commit(ctx) != nil {
		return Attempt{}, ErrUnavailable
	}
	return out, nil
}

// BindSession stores the real native client session exactly once. The runner
// must persist that same session before it exposes question/continuation tools.
func (s *Store) BindSession(ctx context.Context, token, id, session string, epoch int64) (Attempt, error) {
	if !security.ValidUUID(id) || len(session) < 1 || len(session) > 256 || !utf8.ValidString(session) || strings.ContainsAny(session, "\x00\r\n") || epoch < 1 {
		return Attempt{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token, pb.Role_ROLE_EXECUTOR)
	if err != nil {
		return Attempt{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Attempt{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Attempt{}, err
	}
	if _, err = execution(ctx, tx, p, id); err != nil {
		return Attempt{}, err
	}
	a, err := attempt(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Attempt{}, err
	}
	if a.AuthorityEpoch != epoch || (a.Session != "" && a.Session != session) || (a.State != "starting" && a.State != "running") {
		return Attempt{}, ErrInvalid
	}
	changed, err := tx.Exec(ctx, `UPDATE mailbox.work_attempts SET session_id=$2,state='running' WHERE id=$1 AND authority_epoch=$3 AND authority_until>clock_timestamp()`, a.ID, session, epoch)
	if err != nil {
		return Attempt{}, ErrUnavailable
	}
	if changed.RowsAffected() != 1 {
		return Attempt{}, ErrAuthorityLost
	}
	if _, err = tx.Exec(ctx, `UPDATE mailbox.work_assignments SET state='running' WHERE project_id=$1 AND id=$2`, p.ProjectID, id); err != nil {
		return Attempt{}, ErrUnavailable
	}
	a, err = attempt(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Attempt{}, err
	}
	if tx.Commit(ctx) != nil {
		return Attempt{}, ErrUnavailable
	}
	return a, nil
}

// Renew extends a technical authority lease with the exact saved session. No
// total duration cap exists, and an already-lost lease cannot be resurrected.
func (s *Store) Renew(ctx context.Context, token, id, session string, epoch int64) (Attempt, error) {
	if !security.ValidUUID(id) || len(session) > 256 || !utf8.ValidString(session) || strings.ContainsAny(session, "\x00\r\n") || epoch < 1 {
		return Attempt{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token, pb.Role_ROLE_EXECUTOR)
	if err != nil {
		return Attempt{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Attempt{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Attempt{}, err
	}
	if _, err = execution(ctx, tx, p, id); err != nil {
		return Attempt{}, err
	}
	a, err := attempt(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Attempt{}, err
	}
	if a.Session != session || a.AuthorityEpoch != epoch || (a.State != "starting" && a.State != "running" && a.State != "question_wait") {
		return Attempt{}, ErrInvalid
	}
	changed, err := tx.Exec(ctx, `UPDATE mailbox.work_attempts SET authority_until=clock_timestamp()+interval '90 seconds' WHERE id=$1 AND authority_epoch=$2 AND authority_until>clock_timestamp()`, a.ID, epoch)
	if err != nil {
		return Attempt{}, ErrUnavailable
	}
	if changed.RowsAffected() != 1 {
		return Attempt{}, ErrAuthorityLost
	}
	a, err = attempt(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Attempt{}, err
	}
	if tx.Commit(ctx) != nil {
		return Attempt{}, ErrUnavailable
	}
	return a, nil
}
