package contracts

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
)

var ErrForbidden = errors.New("project architect authority required")
var ErrUnavailable = errors.New("work contracts unavailable")

type Store struct {
	Pool       *pgxpool.Pool
	Identities *runneridentity.Store
}
type Record struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"`
	Publisher string    `json:"publisher_runner_id"`
	Content   Content   `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) Publish(ctx context.Context, token string, input Content) (Record, error) {
	content, digest, err := input.Canonical()
	if err != nil {
		return Record{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	principal, err := s.Identities.Authenticate(ctx, token)
	if err != nil || principal.Role != pb.Role_ROLE_ARCHITECT || principal.ProjectID != content.ProjectID {
		return Record{}, ErrForbidden
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	defer rollback(tx)
	var workspace string
	var revision int
	err = tx.QueryRow(ctx, `SELECT p.workspace_id::text,i.current_revision FROM mailbox.orchestration_projects p
 JOIN mailbox.human_intentions i ON i.project_id=p.id WHERE p.id=$1 AND i.id=$2 AND p.active AND p.repository_id=$3
 AND i.state NOT IN ('cancelled','completed') FOR SHARE OF p,i`, content.ProjectID, content.RootID, content.RepositoryID).Scan(&workspace, &revision)
	if err != nil || revision != content.RootRevision {
		return Record{}, ErrStale
	}
	var actor string
	err = tx.QueryRow(ctx, `SELECT id::text FROM mailbox.project_runners WHERE id=$1 AND project_id=$2 AND active AND role='architect'
 AND credential_hash=$3 AND credential_epoch=$4 AND credential_expires_at>clock_timestamp() FOR SHARE`, principal.RunnerID, content.ProjectID, security.Hash(token), principal.CredentialEpoch).Scan(&actor)
	if err != nil {
		return Record{}, ErrForbidden
	}
	raw, _ := json.Marshal(content)
	var out Record
	var saved []byte
	err = tx.QueryRow(ctx, `INSERT INTO mailbox.work_contracts(project_id,root_id,root_revision,repository_id,publisher_runner_id,content_hash,content)
 VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(project_id,content_hash) DO UPDATE SET content_hash=EXCLUDED.content_hash
 RETURNING id::text,content_hash,publisher_runner_id::text,content,created_at`, content.ProjectID, content.RootID, content.RootRevision, content.RepositoryID, actor, digest, raw).Scan(&out.ID, &out.Hash, &out.Publisher, &saved, &out.CreatedAt)
	if err != nil || json.Unmarshal(saved, &out.Content) != nil {
		return Record{}, ErrUnavailable
	}
	// Write one event for the first publication, not an audit on each delivery retry.
	var audits int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM mailbox.audit_events WHERE workspace_id=$1 AND action='contract.published' AND target_id=$2", workspace, out.ID).Scan(&audits); err != nil {
		return Record{}, ErrUnavailable
	}
	if audits == 0 {
		if _, err = tx.Exec(ctx, `INSERT INTO mailbox.audit_events(workspace_id,actor_kind,action,target_id,metadata) VALUES($1,'agent','contract.published',$2,jsonb_build_object('project_id',$3::text,'runner_id',$4::text,'contract_hash',$5::text))`, workspace, out.ID, content.ProjectID, out.Publisher, out.Hash); err != nil {
			return Record{}, ErrUnavailable
		}
	}
	if tx.Commit(ctx) != nil {
		return Record{}, ErrUnavailable
	}
	return out, nil
}
func (s *Store) Get(ctx context.Context, token, project, id string) (Record, error) {
	if !security.ValidUUID(project) || !security.ValidUUID(id) {
		return Record{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.Identities.Authenticate(ctx, token)
	if err != nil || p.ProjectID != project || p.Role != pb.Role_ROLE_ARCHITECT {
		return Record{}, ErrForbidden
	}
	var out Record
	var raw []byte
	err = s.Pool.QueryRow(ctx, `SELECT id::text,content_hash,publisher_runner_id::text,content,created_at FROM mailbox.work_contracts WHERE project_id=$1 AND id=$2`, project, id).Scan(&out.ID, &out.Hash, &out.Publisher, &raw, &out.CreatedAt)
	if err != nil || json.Unmarshal(raw, &out.Content) != nil {
		return Record{}, ErrUnavailable
	}
	return out, nil
}
