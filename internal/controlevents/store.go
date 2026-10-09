// Package controlevents provides transactional, ordered runner event delivery.
package controlevents

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/control"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
	"google.golang.org/protobuf/proto"
)

var ErrInvalid = errors.New("invalid control event")
var ErrForbidden = errors.New("current runner authority required")
var ErrConflict = errors.New("control event key conflict")
var ErrUnavailable = errors.New("control events unavailable")

const MaxPayload = 128 << 10
const MaxPageBytes = 192 << 10

type Store struct {
	Pool       *pgxpool.Pool
	Identities *runneridentity.Store
}
type Event struct {
	ID       string
	Sequence uint64
	Frame    *pb.ServerFrame
}
type principal struct {
	control.Principal
	Repo   repositoryaccess.Repository
	GitHub int64
	Role   string
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) authenticated(ctx context.Context, token string) (principal, error) {
	if s.Pool == nil || s.Identities == nil {
		return principal{}, ErrUnavailable
	}
	p, err := s.Identities.Authenticate(ctx, token)
	if err != nil {
		return principal{}, ErrForbidden
	}
	proof := principal{Principal: p}
	err = s.Pool.QueryRow(ctx, `SELECT r.owner_github_id,r.role,p.repository_id,p.repository_owner_id,p.repository_owner,p.repository_name FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id
 WHERE r.id=$1 AND r.project_id=$2 AND r.active AND p.active AND r.credential_epoch=$3 AND r.credential_hash=$4 AND r.credential_expires_at>clock_timestamp()`, p.RunnerID, p.ProjectID, p.CredentialEpoch, security.Hash(token)).Scan(&proof.GitHub, &proof.Role, &proof.Repo.ID, &proof.Repo.OwnerID, &proof.Repo.Owner, &proof.Repo.Name)
	if err != nil || s.Identities.Authority.Verify(ctx, proof.Repo, proof.GitHub, true) != nil {
		return principal{}, ErrForbidden
	}
	return proof, nil
}
func lockActor(ctx context.Context, tx pgx.Tx, p principal, token string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT r.id::text FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id
 WHERE r.id=$1 AND r.project_id=$2 AND r.active AND p.active AND r.credential_epoch=$3 AND r.credential_hash=$4 AND r.credential_expires_at>clock_timestamp()
 AND r.owner_github_id=$5 AND r.role=$6 AND p.repository_id=$7 AND p.repository_owner_id=$8 AND p.repository_owner=$9 AND p.repository_name=$10 FOR SHARE OF r,p`, p.RunnerID, p.ProjectID, p.CredentialEpoch, security.Hash(token), p.GitHub, p.Role, p.Repo.ID, p.Repo.OwnerID, p.Repo.Owner, p.Repo.Name).Scan(&id)
	if err != nil {
		return ErrForbidden
	}
	return nil
}
func decode(id string, sequence uint64, raw []byte) (Event, error) {
	frame := &pb.ServerFrame{}
	if proto.Unmarshal(raw, frame) != nil || frame.Body == nil || frame.EventId != "" || frame.Cursor != 0 {
		return Event{}, ErrUnavailable
	}
	frame.EventId = id
	frame.Cursor = sequence
	return Event{ID: id, Sequence: sequence, Frame: frame}, nil
}

// PublishTx is called by trusted control services after authorization, in the
// same transaction as the domain change. Per-recipient sequence locks ensure
// commit-order delivery; rollback creates neither a message nor a cursor gap.
func PublishTx(ctx context.Context, tx pgx.Tx, project, runner, key string, frame *pb.ServerFrame) (Event, error) {
	if !security.ValidUUID(project) || !security.ValidUUID(runner) || len(key) < 1 || len(key) > 128 || frame == nil || frame.Body == nil || frame.EventId != "" || frame.Cursor != 0 {
		return Event{}, ErrInvalid
	}
	for _, b := range []byte(key) {
		if b < 32 || b > 126 {
			return Event{}, ErrInvalid
		}
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(frame)
	if err != nil || len(raw) > MaxPayload {
		return Event{}, ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO mailbox.runner_deliveries(project_id,runner_id) SELECT project_id,id FROM mailbox.project_runners WHERE project_id=$1 AND id=$2 AND active ON CONFLICT DO NOTHING`, project, runner)
	if err != nil {
		return Event{}, ErrUnavailable
	}
	var issued uint64
	if err = tx.QueryRow(ctx, `SELECT issued FROM mailbox.runner_deliveries WHERE project_id=$1 AND runner_id=$2 FOR UPDATE`, project, runner).Scan(&issued); err != nil {
		return Event{}, ErrForbidden
	}
	fingerprint := security.Hash(string(raw))
	var id, stored string
	var sequence uint64
	var payload []byte
	err = tx.QueryRow(ctx, `SELECT id::text,sequence,fingerprint,payload FROM mailbox.control_outbox WHERE project_id=$1 AND runner_id=$2 AND event_key=$3`, project, runner, key).Scan(&id, &sequence, &stored, &payload)
	if err == nil {
		if stored != fingerprint {
			return Event{}, ErrConflict
		}
		return decode(id, sequence, payload)
	}
	if err != pgx.ErrNoRows {
		return Event{}, ErrUnavailable
	}
	sequence = issued + 1
	err = tx.QueryRow(ctx, `INSERT INTO mailbox.control_outbox(project_id,runner_id,sequence,event_key,fingerprint,payload) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, project, runner, sequence, key, fingerprint, raw).Scan(&id)
	if err != nil {
		return Event{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE mailbox.runner_deliveries SET issued=$3 WHERE project_id=$1 AND runner_id=$2`, project, runner, sequence); err != nil {
		return Event{}, ErrUnavailable
	}
	return decode(id, sequence, raw)
}

// Page uses the durable server ACK, rather than a caller-supplied skip cursor.
// Sending a page does not acknowledge it; reconnect replays unacknowledged data.
func (s *Store) Page(ctx context.Context, token string) ([]Event, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticated(ctx, token)
	if err != nil {
		return nil, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rollback(tx)
	if err = lockActor(ctx, tx, p, token); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT o.id::text,o.sequence,o.payload FROM mailbox.control_outbox o JOIN mailbox.runner_deliveries d ON d.project_id=o.project_id AND d.runner_id=o.runner_id
 WHERE o.project_id=$1 AND o.runner_id=$2 AND o.sequence>d.acknowledged ORDER BY o.sequence LIMIT 32`, p.ProjectID, p.RunnerID)
	if err != nil {
		return nil, ErrUnavailable
	}
	out := []Event{}
	bytes := 0
	for rows.Next() {
		var id string
		var sequence uint64
		var raw []byte
		if rows.Scan(&id, &sequence, &raw) != nil {
			rows.Close()
			return nil, ErrUnavailable
		}
		if bytes+len(raw) > MaxPageBytes {
			break
		}
		event, e := decode(id, sequence, raw)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, event)
		bytes += len(raw)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || tx.Commit(ctx) != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

// Acknowledge advances exactly one consecutive event and binds its UUID.
// Identical/stale delivery retries are harmless, but skipping data is rejected.
func (s *Store) Acknowledge(ctx context.Context, token string, sequence uint64, id string) error {
	if sequence < 1 || !security.ValidUUID(id) {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticated(ctx, token)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer rollback(tx)
	if err = lockActor(ctx, tx, p, token); err != nil {
		return err
	}
	var ack uint64
	err = tx.QueryRow(ctx, `SELECT acknowledged FROM mailbox.runner_deliveries WHERE project_id=$1 AND runner_id=$2 FOR UPDATE`, p.ProjectID, p.RunnerID).Scan(&ack)
	if err != nil {
		return ErrInvalid
	}
	var actual string
	err = tx.QueryRow(ctx, `SELECT id::text FROM mailbox.control_outbox WHERE project_id=$1 AND runner_id=$2 AND sequence=$3`, p.ProjectID, p.RunnerID, sequence).Scan(&actual)
	if err != nil || actual != id || sequence > ack+1 {
		return ErrInvalid
	}
	if sequence == ack+1 {
		if _, err = tx.Exec(ctx, `UPDATE mailbox.runner_deliveries SET acknowledged=$3 WHERE project_id=$1 AND runner_id=$2`, p.ProjectID, p.RunnerID, sequence); err != nil {
			return ErrUnavailable
		}
	}
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
