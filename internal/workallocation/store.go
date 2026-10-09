// Package workallocation reserves one durable executor slot across projects.
package workallocation

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/control"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
)

var ErrInvalid = errors.New("invalid executor allocation")
var ErrForbidden = errors.New("current project runner authority required")
var ErrUnavailable = errors.New("executor allocation unavailable")
var ErrOccupied = errors.New("executor slot is occupied")
var ErrOffline = errors.New("executor is not available online")
var ErrTaskReserved = errors.New("task has an unresolved assignment")

type Store struct {
	Pool       *pgxpool.Pool
	Identities *runneridentity.Store
}
type Client struct {
	Agent   string   `json:"agent"`
	Version string   `json:"version"`
	Model   *string  `json:"model,omitempty"`
	Effort  *string  `json:"effort,omitempty"`
	Tools   []string `json:"tools"`
}
type Assignment struct {
	ID, Project, Root, Contract, Decision, Task, Executor, State string
	Generation                                                   int64
	Client                                                       Client
	CreatedAt                                                    time.Time
}
type authenticated struct {
	control.Principal
	Repository repositoryaccess.Repository
	GitHubID   int64
	Key        []byte
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (c Client) canonical() ([]byte, string, error) {
	switch c.Agent {
	case "codex", "claude", "copilot", "hermes", "openclaw":
	default:
		return nil, "", ErrInvalid
	}
	if len(c.Version) < 1 || len(c.Version) > 128 || strings.ContainsAny(c.Version, "\x00\r\n") || len(c.Tools) > 64 {
		return nil, "", ErrInvalid
	}
	for _, value := range []*string{c.Model, c.Effort} {
		if value != nil && (len(*value) < 1 || len(*value) > 128 || strings.ContainsAny(*value, "\x00\r\n")) {
			return nil, "", ErrInvalid
		}
	}
	c.Tools = slices.Clone(c.Tools)
	slices.Sort(c.Tools)
	for i, tool := range c.Tools {
		if len(tool) < 1 || len(tool) > 64 || strings.ContainsAny(tool, "\x00\r\n") || (i > 0 && tool == c.Tools[i-1]) {
			return nil, "", ErrInvalid
		}
	}
	raw, _ := json.Marshal(c)
	return raw, security.Hash(string(raw)), nil
}
func (s *Store) authenticate(ctx context.Context, token string, role pb.Role) (authenticated, error) {
	if s.Pool == nil || s.Identities == nil {
		return authenticated{}, ErrUnavailable
	}
	principal, err := s.Identities.Authenticate(ctx, token)
	if err != nil || principal.Role != role {
		return authenticated{}, ErrForbidden
	}
	p := authenticated{Principal: principal}
	err = s.Pool.QueryRow(ctx, `SELECT r.owner_github_id,r.public_key,p.repository_id,p.repository_owner_id,p.repository_owner,p.repository_name
 FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id
 WHERE r.id=$1 AND r.project_id=$2 AND r.active AND p.active AND r.credential_epoch=$3 AND r.credential_hash=$4 AND r.credential_expires_at>clock_timestamp()`, p.RunnerID, p.ProjectID, p.CredentialEpoch, security.Hash(token)).Scan(&p.GitHubID, &p.Key, &p.Repository.ID, &p.Repository.OwnerID, &p.Repository.Owner, &p.Repository.Name)
	if err != nil || s.Identities.Authority.Verify(ctx, p.Repository, p.GitHubID, true) != nil {
		return authenticated{}, ErrForbidden
	}
	return p, nil
}
func actor(ctx context.Context, tx pgx.Tx, p authenticated, token string) error {
	role := "executor"
	if p.Role == pb.Role_ROLE_ARCHITECT {
		role = "architect"
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT r.id::text FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id
 WHERE r.id=$1 AND r.project_id=$2 AND r.active AND p.active AND r.role=$3 AND r.credential_epoch=$4 AND r.credential_hash=$5 AND r.credential_expires_at>clock_timestamp()
 AND r.owner_github_id=$6 AND r.public_key=$7 AND p.repository_id=$8 AND p.repository_owner_id=$9 AND p.repository_owner=$10 AND p.repository_name=$11 FOR SHARE OF r,p`, p.RunnerID, p.ProjectID, role, p.CredentialEpoch, security.Hash(token), p.GitHubID, p.Key, p.Repository.ID, p.Repository.OwnerID, p.Repository.Owner, p.Repository.Name).Scan(&id)
	if err != nil {
		return ErrForbidden
	}
	return nil
}

func read(ctx context.Context, tx pgx.Tx, project, decision string) (Assignment, error) {
	var a Assignment
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT id::text,project_id::text,root_id::text,contract_id::text,decision_id::text,task_id,executor_id::text,state,slot_generation,client,created_at FROM mailbox.work_assignments WHERE project_id=$1 AND decision_id=$2`, project, decision).Scan(&a.ID, &a.Project, &a.Root, &a.Contract, &a.Decision, &a.Task, &a.Executor, &a.State, &a.Generation, &raw, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return a, err
	}
	if err != nil || json.Unmarshal(raw, &a.Client) != nil {
		return a, ErrUnavailable
	}
	return a, nil
}

// Presence renews technical liveness. A caller's available flag cannot release
// a reserved slot or change the effective client of an occupied machine.
func (s *Store) Presence(ctx context.Context, token string, available bool, client Client) error {
	raw, hash, err := client.canonical()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token, pb.Role_ROLE_EXECUTOR)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return err
	}
	// Presence and assignment take the same machine lock even for first contact.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, machine(p.GitHubID, p.Key)); err != nil {
		return ErrUnavailable
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT a.client_hash FROM mailbox.executor_slots s JOIN mailbox.work_assignments a ON a.id=s.assignment_id WHERE s.owner_github_id=$1 AND s.public_key=$2`, p.GitHubID, p.Key).Scan(&existing)
	if err != nil && err != pgx.ErrNoRows {
		return ErrUnavailable
	}
	if err == nil && existing != hash {
		return ErrOccupied
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.executor_presence(project_id,runner_id,credential_epoch,available,client,client_hash,lease_until) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()+interval '90 seconds')
 ON CONFLICT(project_id,runner_id) DO UPDATE SET credential_epoch=EXCLUDED.credential_epoch,available=EXCLUDED.available,client=EXCLUDED.client,client_hash=EXCLUDED.client_hash,lease_until=EXCLUDED.lease_until`, p.ProjectID, p.RunnerID, p.CredentialEpoch, available, raw, hash)
	if err != nil || tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
