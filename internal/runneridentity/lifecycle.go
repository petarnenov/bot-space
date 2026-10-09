package runneridentity

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/petarnenov/bot-space/internal/security"
)

// IdentitySession is implemented by the private journal-backed Session.
type IdentitySession interface {
	Acquire(context.Context, string, func(string) error) (Lease, error)
	Refresh(context.Context, string) (Lease, error)
}
type scopeState struct {
	lease   Lease
	active  bool
	retryAt time.Time
}

// Lifecycle owns project authority, not task deadlines. The supervisor must
// replace connections on OnLease and fence affected work on OnLoss.
type Lifecycle struct {
	Session IdentitySession
	OnLease func(context.Context, Lease) error
	OnLoss  func(string, error)
	mu      sync.Mutex
	scopes  map[string]scopeState
}

func (l *Lifecycle) Start(ctx context.Context, projects []string, open func(string) error) error {
	if l.Session == nil || l.OnLease == nil || len(projects) == 0 {
		return ErrInvalid
	}
	for _, p := range projects {
		if !security.ValidUUID(p) {
			return ErrInvalid
		}
	}
	l.mu.Lock()
	if l.scopes != nil {
		l.mu.Unlock()
		return ErrInvalid
	}
	l.scopes = make(map[string]scopeState)
	l.mu.Unlock()
	for _, p := range projects {
		p = strings.ToLower(p)
		l.mu.Lock()
		_, exists := l.scopes[p]
		l.mu.Unlock()
		if exists {
			continue
		}
		lease, err := l.Session.Acquire(ctx, p, open)
		if err != nil {
			return err
		}
		if lease.ProjectID != p {
			return ErrInvalid
		}
		if err = l.OnLease(ctx, lease); err != nil {
			return err
		}
		l.mu.Lock()
		l.scopes[p] = scopeState{lease: lease, active: true}
		l.mu.Unlock()
	}
	return nil
}

// Tick is called by one lifecycle owner; reads may run concurrently. Any failed
// refresh invalidates the local scope immediately, because a lost HTTP response
// may have already rotated the old credential server-side. Retry uses key proof.
func (l *Lifecycle) Tick(ctx context.Context, now time.Time) {
	l.mu.Lock()
	snapshot := make(map[string]scopeState, len(l.scopes))
	for p, s := range l.scopes {
		snapshot[p] = s
	}
	l.mu.Unlock()
	for p, s := range snapshot {
		if ctx.Err() != nil {
			return
		}
		if s.active && s.lease.ExpiresAt.After(now.Add(3*time.Minute)) {
			continue
		}
		if !s.active && now.Before(s.retryAt) {
			continue
		}
		lease, err := l.Session.Refresh(ctx, p)
		if err == nil && (lease.ProjectID != p || lease.RunnerID != s.lease.RunnerID || lease.Role != s.lease.Role || lease.Epoch <= s.lease.Epoch) {
			err = ErrInvalid
		}
		if err == nil {
			err = l.OnLease(ctx, lease)
		}
		if err != nil {
			l.mu.Lock()
			l.scopes[p] = scopeState{lease: s.lease, active: false, retryAt: now.Add(30 * time.Second)}
			l.mu.Unlock()
			if s.active && l.OnLoss != nil {
				l.OnLoss(p, err)
			}
		} else {
			l.mu.Lock()
			l.scopes[p] = scopeState{lease: lease, active: true}
			l.mu.Unlock()
		}
	}
}
func (l *Lifecycle) Current(project string) (Lease, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.scopes[strings.ToLower(project)]
	if !ok || !s.active || !s.lease.ExpiresAt.After(time.Now()) {
		return Lease{}, ErrUnauthenticated
	}
	return s.lease, nil
}
func (l *Lifecycle) Run(ctx context.Context) error {
	l.mu.Lock()
	started := l.scopes != nil
	l.mu.Unlock()
	if !started {
		return errors.New("identity lifecycle not started")
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			l.Tick(ctx, now)
		}
	}
}
