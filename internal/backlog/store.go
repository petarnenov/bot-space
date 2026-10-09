// Package backlog stores human-originated work with immutable input revisions.
package backlog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/security"
)

var ErrInvalid = errors.New("invalid human intention")
var ErrForbidden = errors.New("project access required")
var ErrConflict = errors.New("intake key or revision conflict")
var ErrUnavailable = errors.New("human backlog unavailable")

type Authority interface {
	Verify(context.Context, repositoryaccess.Repository, int64, bool) error
}
type Store struct {
	Pool      *pgxpool.Pool
	Sessions  *identity.Sessions
	Authority Authority
}
type Input struct {
	ProjectID   string `json:"project_id"`
	Key         string `json:"idempotency_key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Ticket      string `json:"ticket_reference"`
	Priority    int    `json:"priority"`
}
type Intention struct {
	ID                     string    `json:"id"`
	ProjectID              string    `json:"project_id"`
	Creator                string    `json:"creator_user_id"`
	Revision               int       `json:"revision"`
	State                  string    `json:"state"`
	Title                  string    `json:"title"`
	Description            string    `json:"description"`
	Ticket                 string    `json:"ticket_reference"`
	Priority               int       `json:"priority"`
	CreatedAt              time.Time `json:"created_at"`
	Epoch                  int64     `json:"lifecycle_epoch"`
	Archived               bool      `json:"archived"`
	ReconciliationRequired bool      `json:"reconciliation_required"`
}

type Project struct {
	ID, WorkspaceID, Owner, Name string
}

type Progress struct {
	Phase, Outcome                         string
	Decisions, Assignments, Attempts, Done int
}

// Projects returns a bounded list of active projects for which the current
// GitHub user still has explicit repository authority. Workspace membership is
// intentionally not used as project authority.
func (s *Store) Projects(ctx context.Context, secret string) ([]Project, error) {
	if s.Sessions == nil || s.Authority == nil {
		return nil, ErrUnavailable
	}
	session, err := s.Sessions.Authenticate(ctx, secret)
	if err != nil {
		return nil, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.Pool.Query(ctx, `SELECT id::text,workspace_id::text,repository_id,repository_owner_id,repository_owner,repository_name
 FROM mailbox.orchestration_projects WHERE active ORDER BY repository_owner,repository_name,id LIMIT 129`)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	type candidate struct {
		project Project
		repo    repositoryaccess.Repository
	}
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.project.ID, &c.project.WorkspaceID, &c.repo.ID, &c.repo.OwnerID, &c.repo.Owner, &c.repo.Name); err != nil {
			return nil, ErrUnavailable
		}
		c.project.Owner, c.project.Name = c.repo.Owner, c.repo.Name
		candidates = append(candidates, c)
	}
	if rows.Err() != nil || len(candidates) > 128 {
		return nil, ErrUnavailable
	}
	out := []Project{}
	for _, c := range candidates {
		err = s.Authority.Verify(ctx, c.repo, session.User.GitHubID, false)
		if errors.Is(err, repositoryaccess.ErrDenied) {
			continue
		}
		if err != nil {
			return nil, ErrUnavailable
		}
		out = append(out, c.project)
	}
	return out, nil
}

func (s *Store) Project(ctx context.Context, secret, id string) (Project, error) {
	if !security.ValidUUID(id) {
		return Project{}, ErrInvalid
	}
	projects, err := s.Projects(ctx, secret)
	if err != nil {
		return Project{}, err
	}
	for _, project := range projects {
		if strings.EqualFold(project.ID, id) {
			return project, nil
		}
	}
	return Project{}, ErrForbidden
}

// Progress summarizes only durable workflow records attached to an objective.
// It deliberately does not expose runner identities, votes, or private task input.
func (s *Store) Progress(ctx context.Context, secret, project, id string) (Progress, error) {
	if !security.ValidUUID(project) || !security.ValidUUID(id) {
		return Progress{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, _, _, err := s.authorize(ctx, secret, project); err != nil {
		return Progress{}, err
	}
	var out Progress
	err := s.Pool.QueryRow(ctx, `SELECT i.state,
	 (SELECT count(*) FROM mailbox.council_decisions d WHERE d.project_id=i.project_id AND d.root_id=i.id),
	 (SELECT count(*) FROM mailbox.work_assignments a WHERE a.project_id=i.project_id AND a.root_id=i.id),
	 (SELECT count(*) FROM mailbox.work_attempts t JOIN mailbox.work_assignments a ON a.project_id=t.project_id AND a.id=t.assignment_id WHERE a.project_id=i.project_id AND a.root_id=i.id),
	 (SELECT count(*) FROM mailbox.work_assignments a WHERE a.project_id=i.project_id AND a.root_id=i.id AND a.state='completed')
	 FROM mailbox.human_intentions i WHERE i.project_id=$1 AND i.id=$2`, project, id).Scan(&out.Phase, &out.Decisions, &out.Assignments, &out.Attempts, &out.Done)
	if err != nil {
		return Progress{}, ErrForbidden
	}
	switch out.Phase {
	case "completed":
		out.Outcome = "Completed. The recorded workflow contains no separate human-facing result."
	case "cancelled":
		out.Outcome = "Cancelled. Recorded history remains available."
	case "paused":
		out.Outcome = "Paused pending architect reconciliation."
	default:
		out.Outcome = "No final outcome has been recorded yet."
	}
	return out, nil
}

func normalize(in Input) (Input, error) {
	in.ProjectID = strings.ToLower(in.ProjectID)
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if !security.ValidUUID(in.ProjectID) || len(in.Key) < 1 || len(in.Key) > 128 || len(in.Title) < 1 || len(in.Title) > 256 || len(in.Description) < 1 || len(in.Description) > 32768 || len(in.Ticket) > 2048 || in.Priority < 0 || in.Priority > 4 {
		return Input{}, ErrInvalid
	}
	for _, b := range []byte(in.Key) {
		if b < 32 || b > 126 {
			return Input{}, ErrInvalid
		}
	}
	for _, value := range []string{in.Title, in.Description, in.Ticket} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return Input{}, ErrInvalid
		}
	}
	return in, nil
}
func (s *Store) authorize(ctx context.Context, secret, project string) (identity.Session, string, repositoryaccess.Repository, error) {
	if s.Sessions == nil || s.Authority == nil {
		return identity.Session{}, "", repositoryaccess.Repository{}, ErrUnavailable
	}
	session, err := s.Sessions.Authenticate(ctx, secret)
	if err != nil {
		return identity.Session{}, "", repositoryaccess.Repository{}, ErrForbidden
	}
	var workspace string
	var repo repositoryaccess.Repository
	err = s.Pool.QueryRow(ctx, `SELECT workspace_id::text,repository_id,repository_owner_id,repository_owner,repository_name FROM mailbox.orchestration_projects WHERE id=$1 AND active`, project).Scan(&workspace, &repo.ID, &repo.OwnerID, &repo.Owner, &repo.Name)
	if err != nil {
		return identity.Session{}, "", repo, ErrForbidden
	}
	if err = s.Authority.Verify(ctx, repo, session.User.GitHubID, true); err != nil {
		return identity.Session{}, "", repo, ErrForbidden
	}
	return session, workspace, repo, nil
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) Create(ctx context.Context, humanSecret string, input Input) (Intention, error) {
	in, err := normalize(input)
	if err != nil {
		return Intention{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	human, workspace, repo, err := s.authorize(ctx, humanSecret, in.ProjectID)
	if err != nil {
		return Intention{}, err
	}
	raw, _ := json.Marshal(in)
	fingerprint := security.Hash(string(raw))
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	defer rollback(tx)
	var currentUser string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM mailbox.sessions WHERE id_hash=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>clock_timestamp() AND last_seen_at>clock_timestamp()-interval '30 minutes' FOR SHARE`, security.Hash(humanSecret), human.User.ID).Scan(&currentUser)
	if err != nil {
		return Intention{}, ErrForbidden
	}
	var id string
	var stored string
	err = tx.QueryRow(ctx, `INSERT INTO mailbox.human_intentions(workspace_id,project_id,creator_user_id,idempotency_key,fingerprint)
 SELECT workspace_id,id,$2,$3,$4 FROM mailbox.orchestration_projects WHERE id=$1 AND active AND repository_id=$5 AND repository_owner_id=$6
 AND repository_owner=$7 AND repository_name=$8
 ON CONFLICT(project_id,creator_user_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
 RETURNING id::text,fingerprint`, in.ProjectID, human.User.ID, in.Key, fingerprint, repo.ID, repo.OwnerID, repo.Owner, repo.Name).Scan(&id, &stored)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	if stored != fingerprint {
		return Intention{}, ErrConflict
	}
	inserted, err := tx.Exec(ctx, `INSERT INTO mailbox.human_intention_revisions(project_id,intention_id,revision,author_user_id,title,description,ticket_reference,priority) VALUES($1,$2,1,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, in.ProjectID, id, human.User.ID, in.Title, in.Description, in.Ticket, in.Priority)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	if inserted.RowsAffected() == 1 {
		_, err = tx.Exec(ctx, `INSERT INTO mailbox.audit_events(workspace_id,actor_kind,actor_user_id,action,target_id,metadata) VALUES($1,'human',$2,'intention.created',$3,jsonb_build_object('project_id',$4::text,'revision',1))`, workspace, human.User.ID, id, in.ProjectID)
		if err != nil {
			return Intention{}, ErrUnavailable
		}
	}
	out, err := read(ctx, tx, in.ProjectID, id, 1)
	if err != nil {
		return Intention{}, err
	}
	if tx.Commit(ctx) != nil {
		return Intention{}, ErrUnavailable
	}
	return out, nil
}

type reader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func read(ctx context.Context, q reader, project, id string, revision int) (Intention, error) {
	var out Intention
	err := q.QueryRow(ctx, `SELECT i.id::text,i.project_id::text,i.creator_user_id::text,r.revision,i.state,r.title,r.description,r.ticket_reference,r.priority,i.created_at,i.lifecycle_epoch,i.archived,i.reconciliation_required
 FROM mailbox.human_intentions i JOIN mailbox.human_intention_revisions r ON r.project_id=i.project_id AND r.intention_id=i.id
 WHERE i.project_id=$1 AND i.id=$2 AND r.revision=$3`, project, id, revision).Scan(&out.ID, &out.ProjectID, &out.Creator, &out.Revision, &out.State, &out.Title, &out.Description, &out.Ticket, &out.Priority, &out.CreatedAt, &out.Epoch, &out.Archived, &out.ReconciliationRequired)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	return out, nil
}

func (s *Store) Revise(ctx context.Context, secret, id string, expected int, input Input) (Intention, error) {
	in, err := normalize(input)
	if err != nil || !security.ValidUUID(id) || expected < 1 {
		return Intention{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	human, workspace, repo, err := s.authorize(ctx, secret, in.ProjectID)
	if err != nil {
		return Intention{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	defer rollback(tx)
	var currentUser string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM mailbox.sessions WHERE id_hash=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>clock_timestamp() AND last_seen_at>clock_timestamp()-interval '30 minutes' FOR SHARE`, security.Hash(secret), human.User.ID).Scan(&currentUser)
	if err != nil {
		return Intention{}, ErrForbidden
	}
	var current int
	var creator string
	err = tx.QueryRow(ctx, `SELECT i.current_revision,i.creator_user_id::text FROM mailbox.human_intentions i
 JOIN mailbox.orchestration_projects p ON p.id=i.project_id WHERE i.id=$1 AND i.project_id=$2 AND p.active AND NOT i.archived AND i.state NOT IN ('cancelled','completed')
 AND p.repository_id=$3 AND p.repository_owner_id=$4 AND p.repository_owner=$5 AND p.repository_name=$6 FOR UPDATE OF i FOR SHARE OF p`, id, in.ProjectID, repo.ID, repo.OwnerID, repo.Owner, repo.Name).Scan(&current, &creator)
	if err != nil || creator != human.User.ID {
		return Intention{}, ErrForbidden
	}
	if current != expected {
		return Intention{}, ErrConflict
	}
	next := current + 1
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.human_intention_revisions(project_id,intention_id,revision,author_user_id,title,description,ticket_reference,priority) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, in.ProjectID, id, next, human.User.ID, in.Title, in.Description, in.Ticket, in.Priority)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE mailbox.human_intentions SET current_revision=$3 WHERE id=$1 AND project_id=$2`, id, in.ProjectID, next); err != nil {
		return Intention{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.audit_events(workspace_id,actor_kind,actor_user_id,action,target_id,metadata) VALUES($1,'human',$2,'intention.revised',$3,jsonb_build_object('project_id',$4::text,'revision',$5::integer))`, workspace, human.User.ID, id, in.ProjectID, next)
	if err != nil {
		return Intention{}, ErrUnavailable
	}
	out, err := read(ctx, tx, in.ProjectID, id, next)
	if err != nil {
		return Intention{}, err
	}
	if tx.Commit(ctx) != nil {
		return Intention{}, ErrUnavailable
	}
	return out, nil
}
func (s *Store) Get(ctx context.Context, secret, project, id string, revision int) (Intention, error) {
	if !security.ValidUUID(project) || !security.ValidUUID(id) || revision < 0 {
		return Intention{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, _, _, err := s.authorize(ctx, secret, project); err != nil {
		return Intention{}, err
	}
	if revision == 0 {
		if err := s.Pool.QueryRow(ctx, `SELECT current_revision FROM mailbox.human_intentions WHERE id=$1 AND project_id=$2`, id, project).Scan(&revision); err != nil {
			return Intention{}, ErrForbidden
		}
	}
	return read(ctx, s.Pool, project, id, revision)
}

// List returns bounded pages in stable creation order. Cursor UUIDs are scoped
// by the query's project; a foreign cursor cannot disclose another backlog.
func (s *Store) List(ctx context.Context, secret, project, after string) ([]Intention, error) {
	if !security.ValidUUID(project) || (after != "" && !security.ValidUUID(after)) {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, _, _, err := s.authorize(ctx, secret, project); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT i.id::text,i.project_id::text,i.creator_user_id::text,r.revision,i.state,r.title,r.description,r.ticket_reference,r.priority,i.created_at,i.lifecycle_epoch,i.archived,i.reconciliation_required
 FROM mailbox.human_intentions i JOIN mailbox.human_intention_revisions r ON r.project_id=i.project_id AND r.intention_id=i.id AND r.revision=i.current_revision
 WHERE i.project_id=$1 AND NOT i.archived AND ($2='' OR (i.created_at,i.id)>(SELECT created_at,id FROM mailbox.human_intentions WHERE project_id=$1 AND id=NULLIF($2,'')::uuid))
 ORDER BY i.created_at,i.id LIMIT 50`, project, after)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Intention{}
	for rows.Next() {
		var item Intention
		if err = rows.Scan(&item.ID, &item.ProjectID, &item.Creator, &item.Revision, &item.State, &item.Title, &item.Description, &item.Ticket, &item.Priority, &item.CreatedAt, &item.Epoch, &item.Archived, &item.ReconciliationRequired); err != nil {
			return nil, ErrUnavailable
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}
