package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/control"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type identityStreamBackend struct {
	events  chan *pb.ServerFrame
	applied atomic.Int64
}

func (b *identityStreamBackend) Presence(context.Context, control.Principal, *pb.PresenceRequest) error {
	return nil
}
func (b *identityStreamBackend) Inspect(context.Context, control.Principal, *pb.InspectRequest) (*pb.InspectResponse, error) {
	return &pb.InspectResponse{State: "test"}, nil
}
func (b *identityStreamBackend) Apply(context.Context, control.Principal, *pb.RunnerFrame) error {
	b.applied.Add(1)
	return nil
}
func (b *identityStreamBackend) Pull(ctx context.Context, _ control.Principal, _ uint64) (*pb.ServerFrame, error) {
	select {
	case event := <-b.events:
		return event, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// This fixture uses real PostgreSQL credentials and verified TLS. Its events
// are synthetic: durable delivery is verified separately from revocation.
func assertRunnerRevokedOnOpenTLSStream(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *runneridentity.Store, credential runneridentity.Credential, pair tls.Certificate) {
	t.Helper()
	backend := &identityStreamBackend{events: make(chan *pb.ServerFrame, 1)}
	rpc, err := control.New(store.Authenticate, backend, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})))
	if err != nil {
		t.Fatal(err)
	}
	defer rpc.Stop()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go rpc.Serve(listener)
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	authenticated := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+credential.Token))
	stream, err := pb.NewControlClient(conn).Connect(authenticated)
	if err != nil {
		t.Fatal(err)
	}
	if err = stream.Send(&pb.RunnerFrame{Body: &pb.RunnerFrame_Resume{Resume: &pb.Resume{}}}); err != nil {
		t.Fatal(err)
	}
	backend.events <- &pb.ServerFrame{EventId: "synthetic-event", Cursor: 1, Body: &pb.ServerFrame_Receipt{Receipt: &pb.Receipt{RequestId: "ready"}}}
	if _, err = stream.Recv(); err != nil {
		t.Fatal("initial PostgreSQL-authenticated TLS stream failed", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE mailbox.project_runners SET active=false WHERE id=$1", credential.RunnerID); err != nil {
		t.Fatal(err)
	}
	if err = stream.Send(&pb.RunnerFrame{RequestId: "post-revocation", Body: &pb.RunnerFrame_Ack{Ack: &pb.Ack{Cursor: 1, EventId: "synthetic-event"}}}); err != nil {
		t.Fatal(err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.Unauthenticated || backend.applied.Load() != 0 {
		t.Fatal("revoked stream applied privileged frame", err)
	}
}
