package runneridentity

import (
	"context"
	"time"

	"github.com/petarnenov/bot-space/internal/repositoryaccess"
)

// ConfigureProject is for the server operator bootstrap command, not an agent
// RPC. Its repository must come from authenticated Resolve. It creates no user
// memberships or per-runner grants.
func (s *Store) ConfigureProject(ctx context.Context, workspace string, repo repositoryaccess.Repository) (string, error) {
	if workspace == "" || repo.ID < 1 || repo.OwnerID < 1 || repo.Owner == "" || repo.Name == "" {
		return "", ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name)
 SELECT id,$2,$3,$4,$5 FROM mailbox.workspaces WHERE slug=$1
 ON CONFLICT(workspace_id,repository_id) DO UPDATE SET repository_id=EXCLUDED.repository_id
 WHERE mailbox.orchestration_projects.active AND mailbox.orchestration_projects.repository_owner_id=EXCLUDED.repository_owner_id
 AND mailbox.orchestration_projects.repository_owner=EXCLUDED.repository_owner AND mailbox.orchestration_projects.repository_name=EXCLUDED.repository_name
 RETURNING id::text`, workspace, repo.ID, repo.OwnerID, repo.Owner, repo.Name).Scan(&id)
	if err != nil {
		return "", ErrUnavailable
	}
	return id, nil
}
