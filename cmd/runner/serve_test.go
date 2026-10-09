//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"io"
	"strings"
	"testing"
	"time"
)

type startupSession struct{ acquired []string }

func (s *startupSession) Acquire(_ context.Context, p string, _ func(string) error) (runneridentity.Lease, error) {
	s.acquired = append(s.acquired, p)
	return runneridentity.Lease{Credential: runneridentity.Credential{RunnerID: p, ProjectID: p, Role: runneridentity.Executor, Epoch: 1, Token: "private-sentinel", ExpiresAt: time.Now().Add(15 * time.Minute)}}, nil
}
func (s *startupSession) Refresh(context.Context, string) (runneridentity.Lease, error) {
	return runneridentity.Lease{}, errors.New("unexpected refresh")
}

type testConnection struct{ closed bool }

func (c *testConnection) Close() error { c.closed = true; return nil }
func TestServeAutomaticallyAcquiresScopesAndClosesConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	projects := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	session := &startupSession{}
	var out bytes.Buffer
	var connections []*testConnection
	connector := func(context.Context, runneridentity.Lease) (io.Closer, error) {
		c := &testConnection{}
		connections = append(connections, c)
		if len(connections) == 2 {
			cancel()
		}
		return c, nil
	}
	if err := serveIdentity(ctx, session, projects, nil, &out, connector); err != nil {
		t.Fatal(err)
	}
	if len(session.acquired) != 2 || len(connections) != 2 {
		t.Fatal("startup omitted a project")
	}
	for _, c := range connections {
		if !c.closed {
			t.Fatal("shutdown left connection open")
		}
	}
	if strings.Contains(out.String(), "private-sentinel") || strings.Count(out.String(), "identity_ready") != 2 {
		t.Fatal("unsafe or incomplete status")
	}
}
func TestServeClosesEarlierScopesAfterStartupFailure(t *testing.T) {
	session := &startupSession{}
	connection := &testConnection{}
	calls := 0
	connect := func(context.Context, runneridentity.Lease) (io.Closer, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("probe failed")
		}
		return connection, nil
	}
	err := serveIdentity(context.Background(), session, []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}, nil, &bytes.Buffer{}, connect)
	if err == nil || !connection.closed {
		t.Fatal("failed startup retained earlier authority connection")
	}
}
