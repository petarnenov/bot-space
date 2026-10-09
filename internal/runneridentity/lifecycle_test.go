package runneridentity

import (
	"context"
	"testing"
	"time"
)

type lifecycleSession struct {
	leases map[string]Lease
	fail   map[string]bool
	calls  map[string]int
}

func (s *lifecycleSession) Acquire(_ context.Context, p string, _ func(string) error) (Lease, error) {
	return s.leases[p], nil
}
func (s *lifecycleSession) Refresh(_ context.Context, p string) (Lease, error) {
	s.calls[p]++
	if s.fail[p] {
		return Lease{}, ErrUnavailable
	}
	lease := s.leases[p]
	lease.Epoch++
	lease.ExpiresAt = time.Now().Add(15 * time.Minute)
	s.leases[p] = lease
	return lease, nil
}
func TestLifecycleInvalidatesOnlyLostScopeAndRecoversWithNewEpoch(t *testing.T) {
	other := "22222222-2222-4222-8222-222222222222"
	now := time.Now()
	one := Lease{Credential: Credential{RunnerID: project, ProjectID: project, Role: Executor, Epoch: 1, ExpiresAt: now.Add(time.Minute)}}
	two := Lease{Credential: Credential{RunnerID: other, ProjectID: other, Role: Executor, Epoch: 1, ExpiresAt: now.Add(15 * time.Minute)}}
	session := &lifecycleSession{map[string]Lease{project: one, other: two}, map[string]bool{}, map[string]int{}}
	losses := 0
	updates := 0
	lifecycle := Lifecycle{Session: session, OnLease: func(context.Context, Lease) error { updates++; return nil }, OnLoss: func(p string, _ error) {
		if p != project {
			t.Error("unrelated scope lost")
		}
		losses++
	}}
	if err := lifecycle.Start(context.Background(), []string{project, other, project}, nil); err != nil {
		t.Fatal(err)
	}
	if updates != 2 {
		t.Fatal("duplicate project started")
	}
	session.fail[project] = true
	lifecycle.Tick(context.Background(), now)
	if _, err := lifecycle.Current(project); err != ErrUnauthenticated || losses != 1 {
		t.Fatal("failed refresh kept local authority")
	}
	if _, err := lifecycle.Current(other); err != nil {
		t.Fatal("one failure stopped unrelated scope")
	}
	lifecycle.Tick(context.Background(), now.Add(time.Second))
	if session.calls[project] != 1 {
		t.Fatal("retry backoff ignored")
	}
	session.fail[project] = false
	lifecycle.Tick(context.Background(), now.Add(31*time.Second))
	restored, err := lifecycle.Current(project)
	if err != nil || restored.Epoch != 2 || updates != 3 {
		t.Fatal("scope did not recover with new epoch", err)
	}
}
