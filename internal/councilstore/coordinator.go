package councilstore

import (
	"context"
	"slices"
	"time"

	"github.com/petarnenov/bot-space/internal/council"
	"github.com/petarnenov/bot-space/internal/security"
)

// Coordinate leases only proposal-writing authority. It never expires a round,
// supplies a vote, removes offline members or creates an execution deadline.
func (s *Store) Coordinate(ctx context.Context, token, id string) (Lease, error) {
	if !security.ValidUUID(id) {
		return Lease{}, council.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token)
	if err != nil {
		return Lease{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Lease{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Lease{}, err
	}
	r, _, err := load(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Lease{}, err
	}
	if err = current(ctx, tx, r); err != nil {
		return Lease{}, err
	}
	if !slices.Contains(r.Snapshot.Members, p.RunnerID) {
		return Lease{}, council.ErrMember
	}
	_, material, err := source(ctx, tx, p.ProjectID, r.Contract)
	if err != nil || material.RootRevision != r.Material.RootRevision {
		return Lease{}, ErrStale
	}
	if r.Snapshot.Status == council.Accepted || r.Snapshot.Status == council.Blocked {
		return Lease{}, council.ErrState
	}
	var lease Lease
	err = tx.QueryRow(ctx, `INSERT INTO mailbox.council_coordinators(project_id,decision_id,runner_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '30 seconds')
 ON CONFLICT(project_id,decision_id) DO UPDATE SET runner_id=EXCLUDED.runner_id,
 epoch=CASE WHEN council_coordinators.expires_at<=clock_timestamp() THEN council_coordinators.epoch+1 ELSE council_coordinators.epoch END,
 expires_at=EXCLUDED.expires_at WHERE council_coordinators.runner_id=EXCLUDED.runner_id OR council_coordinators.expires_at<=clock_timestamp()
 RETURNING epoch,expires_at`, p.ProjectID, id, p.RunnerID).Scan(&lease.Epoch, &lease.ExpiresAt)
	if err != nil {
		return Lease{}, ErrStale
	}
	if tx.Commit(ctx) != nil {
		return Lease{}, ErrUnavailable
	}
	return lease, nil
}

func (s *Store) Revise(ctx context.Context, token, id string, epoch int64, round int, hash string, proposal council.Proposal) (Record, error) {
	if !security.ValidUUID(id) || epoch < 1 {
		return Record{}, council.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token)
	if err != nil {
		return Record{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Record{}, err
	}
	r, d, err := load(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Record{}, err
	}
	if err = current(ctx, tx, r); err != nil {
		return Record{}, err
	}
	if err = s.proposal(ctx, tx, p, r.Contract, r.Snapshot.Kind, r.Subject, proposal); err != nil {
		return Record{}, err
	}
	_, material, err := source(ctx, tx, p.ProjectID, r.Contract)
	if err != nil || material.RootRevision != r.Material.RootRevision {
		return Record{}, ErrStale
	}
	var current bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.council_coordinators WHERE project_id=$1 AND decision_id=$2 AND runner_id=$3 AND epoch=$4 AND expires_at>clock_timestamp())`, p.ProjectID, id, p.RunnerID, epoch).Scan(&current)
	if err != nil || !current {
		return Record{}, ErrStale
	}
	last := r.Snapshot.Rounds[len(r.Snapshot.Rounds)-1]
	if last.Number != round || last.Hash != hash {
		return Record{}, council.ErrStale
	}
	if err = d.Revise(proposal); err != nil {
		return Record{}, err
	}
	if err = persist(ctx, tx, r, d); err != nil {
		return Record{}, err
	}
	r.Snapshot = d.Snapshot()
	if tx.Commit(ctx) != nil {
		return Record{}, ErrUnavailable
	}
	return r, nil
}
