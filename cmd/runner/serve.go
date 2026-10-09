//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/runneridentity"
)

type identityConnector func(context.Context, runneridentity.Lease) (io.Closer, error)

func connectIdentity(ctx context.Context, lease runneridentity.Lease) (io.Closer, error) {
	conn, native, err := runneridentity.DialNative(lease)
	if err != nil {
		return nil, err
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	own, err := native.Inspect(check, &pb.InspectRequest{ResourceId: lease.RunnerID})
	if err != nil || own.GetResourceId() != lease.RunnerID || own.GetState() != "enrolled" {
		conn.Close()
		return nil, errors.New("native identity verification failed")
	}
	return conn, nil
}

// serveIdentity is the automatic startup/renewal owner. Durable subscriptions
// and role work dispatch attach to these project connections in later tasks.
func serveIdentity(ctx context.Context, session runneridentity.IdentitySession, scopes []string, open func(string) error, out io.Writer, connect identityConnector) error {
	connections := map[string]io.Closer{}
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
	}()
	lifecycle := runneridentity.Lifecycle{Session: session}
	lifecycle.OnLease = func(ctx context.Context, lease runneridentity.Lease) error {
		conn, err := connect(ctx, lease)
		if err != nil {
			return err
		}
		if old := connections[lease.ProjectID]; old != nil {
			old.Close()
		}
		connections[lease.ProjectID] = conn
		// Tokens and keys never enter console status output.
		if err = json.NewEncoder(out).Encode(struct {
			State   string              `json:"state"`
			Runner  string              `json:"runner_id"`
			Project string              `json:"project_id"`
			Role    runneridentity.Role `json:"role"`
			Epoch   uint64              `json:"epoch"`
		}{"identity_ready", lease.RunnerID, lease.ProjectID, lease.Role, lease.Epoch}); err != nil {
			return errors.New("runner status output unavailable")
		}
		return nil
	}
	lifecycle.OnLoss = func(project string, _ error) {
		if conn := connections[project]; conn != nil {
			conn.Close()
			delete(connections, project)
		}
		json.NewEncoder(out).Encode(map[string]string{"state": "authority_unavailable", "project_id": project})
	}
	if err := lifecycle.Start(ctx, scopes, open); err != nil {
		return err
	}
	return lifecycle.Run(ctx)
}
