//go:build darwin || linux

package runneridentity

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/petarnenov/bot-space/internal/security"
)

// Session joins private state and authenticated startup/refresh. It must remain
// owned by the supervisor; models receive only role/task-scoped bridge handles.
type Session struct {
	mu    sync.Mutex
	state *State
	api   *Client
}

func NewSession(state *State, api *Client) (*Session, error) {
	if state == nil || api == nil {
		return nil, ErrInvalid
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.dir == nil || state.profile.Origin != api.origin {
		return nil, ErrInvalid
	}
	return &Session{state: state, api: api}, nil
}

// Acquire revalidates existing GitHub-bound identity through key-proved refresh.
// New project scopes perform OAuth enrollment; one key remains shared across
// scopes. A revoked scope is not silently replaced or rebound to another user.
func (s *Session) Acquire(ctx context.Context, project string, open func(string) error) (Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !security.ValidUUID(project) {
		return Lease{}, ErrInvalid
	}
	project = strings.ToLower(project)
	current, err := s.state.LoadLease(project)
	var lease Lease
	if err == nil {
		lease, err = s.api.Refresh(ctx, current, s.state.SigningKey())
	} else if errors.Is(err, os.ErrNotExist) {
		lease, err = s.api.Enroll(ctx, project, s.state.profile.Role, s.state.SigningKey(), open)
	}
	if err != nil {
		return Lease{}, err
	}
	if err = s.state.SaveLease(lease); err != nil {
		return Lease{}, err
	}
	return lease, nil
}
func (s *Session) Refresh(ctx context.Context, project string) (Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.state.LoadLease(project)
	if err != nil {
		return Lease{}, err
	}
	lease, err := s.api.Refresh(ctx, current, s.state.SigningKey())
	if err != nil {
		return Lease{}, err
	}
	if err = s.state.SaveLease(lease); err != nil {
		return Lease{}, err
	}
	return lease, nil
}
