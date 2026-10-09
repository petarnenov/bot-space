package controlevents

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/security"
)

func (s *Store) ValidateResume(ctx context.Context, token string, cursor uint64) error {
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
	err = tx.QueryRow(ctx, `SELECT acknowledged FROM mailbox.runner_deliveries WHERE project_id=$1 AND runner_id=$2`, p.ProjectID, p.RunnerID).Scan(&ack)
	if err != nil && err != pgx.ErrNoRows {
		return ErrUnavailable
	}
	if cursor > ack {
		return ErrInvalid
	}
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}

// Next uses the stream's server-maintained sent cursor only after validated
// resume. Idle checks query current activation/epoch without polling GitHub on
// every tick; project access is force-refreshed periodically and before delivery.
func (s *Store) Next(ctx context.Context, token string, cursor uint64) (*pb.ServerFrame, error) {
	p, err := s.authenticated(ctx, token)
	if err != nil {
		return nil, err
	}
	refreshed := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var current, queued bool
		err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.project_runners r JOIN mailbox.orchestration_projects p ON p.id=r.project_id
 WHERE r.id=$1 AND r.project_id=$2 AND r.active AND p.active AND r.credential_epoch=$3 AND r.credential_hash=$4 AND r.credential_expires_at>clock_timestamp()
 AND r.owner_github_id=$5 AND p.repository_id=$6 AND p.repository_owner_id=$7 AND p.repository_owner=$8 AND p.repository_name=$9),
 EXISTS(SELECT 1 FROM mailbox.control_outbox o JOIN mailbox.runner_deliveries d ON d.project_id=o.project_id AND d.runner_id=o.runner_id WHERE o.project_id=$2 AND o.runner_id=$1 AND o.sequence>GREATEST(d.acknowledged,$10))`, p.RunnerID, p.ProjectID, p.CredentialEpoch, security.Hash(token), p.GitHub, p.Repo.ID, p.Repo.OwnerID, p.Repo.Owner, p.Repo.Name, cursor).Scan(&current, &queued)
		if err != nil {
			return nil, ErrUnavailable
		}
		if !current {
			return nil, ErrForbidden
		}
		if queued {
			page, e := s.Page(ctx, token)
			if e != nil {
				return nil, e
			}
			for _, event := range page {
				if event.Sequence > cursor {
					return event.Frame, nil
				}
			}
		}
		if time.Since(refreshed) >= 30*time.Second {
			fresh, e := s.authenticated(ctx, token)
			if e != nil || fresh.Principal != p.Principal {
				return nil, ErrForbidden
			}
			p = fresh
			refreshed = time.Now()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
