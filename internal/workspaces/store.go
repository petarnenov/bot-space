// Package workspaces enforces invitation-only team access and role invariants.
package workspaces

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not_found")
	ErrConflict    = errors.New("conflict")
	ErrInvalid     = errors.New("invalid_argument")
	ErrUnavailable = errors.New("temporarily_unavailable")
)

const OperationTimeout = 5 * time.Second

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type Store struct{ Pool *pgxpool.Pool }
type User struct {
	ID       string
	GitHubID int64
	Username string
}
type Workspace struct{ ID, Slug, Role string }
type Member struct {
	User
	Role string
}

func ValidSlug(slug string) bool { return slugPattern.MatchString(slug) }

func (s *Store) Bootstrap(ctx context.Context, githubID int64, slug string) (Workspace, error) {
	if githubID < 1 || !ValidSlug(slug) {
		return Workspace{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Workspace{}, ErrUnavailable
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "bootstrap:"+slug); err != nil {
		return Workspace{}, ErrUnavailable
	}
	var userID string
	if err = tx.QueryRow(ctx, "INSERT INTO mailbox.users (github_id) VALUES ($1) ON CONFLICT (github_id) DO UPDATE SET github_id=EXCLUDED.github_id RETURNING id::text", githubID).Scan(&userID); err != nil {
		return Workspace{}, ErrUnavailable
	}
	var w Workspace
	var bootstrapID int64
	err = tx.QueryRow(ctx, "SELECT w.id::text,w.slug,COALESCE(m.role,''),w.bootstrap_github_id FROM mailbox.workspaces w LEFT JOIN mailbox.memberships m ON m.workspace_id=w.id AND m.user_id=$2 WHERE w.slug=$1", slug, userID).Scan(&w.ID, &w.Slug, &w.Role, &bootstrapID)
	if err == nil {
		if w.Role != "owner" || bootstrapID != githubID {
			return Workspace{}, ErrConflict
		}
		return w, commit(ctx, tx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrUnavailable
	}
	if err = tx.QueryRow(ctx, "INSERT INTO mailbox.workspaces (slug,bootstrap_github_id) VALUES ($1,$2) RETURNING id::text,slug", slug, githubID).Scan(&w.ID, &w.Slug); err != nil {
		return Workspace{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "INSERT INTO mailbox.memberships (workspace_id,user_id,role) VALUES ($1,$2,'owner')", w.ID, userID); err != nil {
		return Workspace{}, ErrUnavailable
	}
	if err = audit(ctx, tx, w.ID, "", "workspace_bootstrapped", userID); err != nil {
		return Workspace{}, err
	}
	w.Role = "owner"
	return w, commit(ctx, tx)
}

func (s *Store) List(ctx context.Context, userID string) ([]Workspace, error) {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	rows, err := s.Pool.Query(ctx, "SELECT w.id::text,w.slug,m.role FROM mailbox.workspaces w JOIN mailbox.memberships m ON m.workspace_id=w.id WHERE m.user_id=$1 ORDER BY w.slug LIMIT 100", userID)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Workspace{}
	for rows.Next() {
		var w Workspace
		if rows.Scan(&w.ID, &w.Slug, &w.Role) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, w)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

func (s *Store) Members(ctx context.Context, workspaceID, actorID string) ([]Member, error) {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rollback(tx)
	if _, err := lockActor(ctx, tx, workspaceID, actorID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT u.id::text,u.github_id,u.username,m.role FROM mailbox.memberships m JOIN mailbox.users u ON u.id=m.user_id WHERE m.workspace_id=$1 ORDER BY u.github_id LIMIT 100", workspaceID)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if rows.Scan(&m.ID, &m.GitHubID, &m.Username, &m.Role) != nil {
			return nil, ErrUnavailable
		}
		out = append(out, m)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	rows.Close()
	return out, commit(ctx, tx)
}

func (s *Store) SetRole(ctx context.Context, workspaceID, actorID, targetID, role string) error {
	if role != "owner" && role != "admin" && role != "member" {
		return ErrInvalid
	}
	return s.mutateMember(ctx, workspaceID, actorID, targetID, role, false)
}

func (s *Store) Remove(ctx context.Context, workspaceID, actorID, targetID string) error {
	return s.mutateMember(ctx, workspaceID, actorID, targetID, "", true)
}

func (s *Store) mutateMember(ctx context.Context, workspaceID, actorID, targetID, role string, remove bool) error {
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer rollback(tx)
	actor, err := lockActor(ctx, tx, workspaceID, actorID)
	if err != nil {
		return err
	}
	if actor != "owner" && actor != "admin" {
		return ErrForbidden
	}
	var current string
	if err := tx.QueryRow(ctx, "SELECT role FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2", workspaceID, targetID).Scan(&current); err != nil {
		return ErrNotFound
	}
	if actor == "admin" && (current == "owner" || role == "owner") {
		return ErrForbidden
	}
	if current == "owner" && (remove || role != "owner") {
		var count int
		if tx.QueryRow(ctx, "SELECT count(*) FROM mailbox.memberships WHERE workspace_id=$1 AND role='owner'", workspaceID).Scan(&count) != nil {
			return ErrUnavailable
		}
		if count <= 1 {
			return ErrConflict
		}
	}
	action := "member_role_changed"
	if remove {
		if _, err = tx.Exec(ctx, "SELECT set_config('mailbox.audit_actor',$1,true)", actorID); err != nil {
			return ErrUnavailable
		}
		_, err = tx.Exec(ctx, "DELETE FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2", workspaceID, targetID)
		action = "member_removed"
	} else {
		if current == role {
			return commit(ctx, tx)
		}
		_, err = tx.Exec(ctx, "UPDATE mailbox.memberships SET role=$3 WHERE workspace_id=$1 AND user_id=$2", workspaceID, targetID, role)
	}
	if err != nil {
		return ErrUnavailable
	}
	metadata := map[string]string{}
	if !remove {
		metadata["role_from"] = current
		metadata["role_to"] = role
	}
	if err = audit(ctx, tx, workspaceID, actorID, action, targetID, metadata); err != nil {
		return err
	}
	return commit(ctx, tx)
}

func lockActor(ctx context.Context, tx pgx.Tx, workspaceID, actorID string) (string, error) {
	var id string
	if tx.QueryRow(ctx, "SELECT id::text FROM mailbox.workspaces WHERE id=$1 FOR UPDATE", workspaceID).Scan(&id) != nil {
		return "", ErrNotFound
	}
	var role string
	if tx.QueryRow(ctx, "SELECT role FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2", workspaceID, actorID).Scan(&role) != nil {
		return "", ErrForbidden
	}
	return role, nil
}

func audit(ctx context.Context, tx pgx.Tx, workspaceID, actorID, action, targetID string, extra ...map[string]string) error {
	kind := "human"
	var actor any = actorID
	if actorID == "" {
		kind = "system"
		actor = nil
	}
	metadata := map[string]string{}
	if len(extra) > 0 {
		metadata = extra[0]
	}
	_, err := tx.Exec(ctx, "INSERT INTO mailbox.audit_events (workspace_id,actor_kind,actor_user_id,action,target_id,metadata) VALUES ($1,$2,$3,$4,$5,$6)", workspaceID, kind, actor, action, targetID, metadata)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func commit(ctx context.Context, tx pgx.Tx) error {
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
