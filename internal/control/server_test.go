package control

import (
	"context"
	"crypto/tls"
	"errors"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fixtureBackend struct {
	events  chan *pb.ServerFrame
	applied chan *pb.RunnerFrame
	cursors chan uint64
}

func (b *fixtureBackend) Presence(context.Context, Principal, *pb.PresenceRequest) error { return nil }
func (b *fixtureBackend) Inspect(_ context.Context, _ Principal, r *pb.InspectRequest) (*pb.InspectResponse, error) {
	if r.ResourceId == "private" {
		return nil, status.Error(codes.PermissionDenied, "secret body sentinel")
	}
	return &pb.InspectResponse{ResourceId: r.ResourceId, State: "pending"}, nil
}
func (b *fixtureBackend) Apply(ctx context.Context, _ Principal, r *pb.RunnerFrame) error {
	select {
	case b.applied <- r:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (b *fixtureBackend) Pull(ctx context.Context, _ Principal, cursor uint64) (*pb.ServerFrame, error) {
	select {
	case b.cursors <- cursor:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case event := <-b.events:
		return event, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func setup(t *testing.T, role pb.Role) (pb.ControlClient, *fixtureBackend, *atomic.Bool, context.Context) {
	t.Helper()
	revoked := &atomic.Bool{}
	b := &fixtureBackend{make(chan *pb.ServerFrame, 4), make(chan *pb.RunnerFrame, 4), make(chan uint64, 8)}
	auth := func(_ context.Context, token string) (Principal, error) {
		if token != "test-only-token" || revoked.Load() {
			return Principal{}, errors.New("private auth sentinel")
		}
		return Principal{RunnerID: "runner", ProjectID: "project", Role: role, CredentialEpoch: 1}, nil
	}
	certSource := httptest.NewTLSServer(http.NotFoundHandler())
	cert := certSource.TLS.Certificates[0]
	certSource.Close()
	server, err := New(auth, b, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})))
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	go server.Serve(listener)
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}))) // Test-only self-signed certificate.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(); server.Stop(); listener.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return pb.NewControlClient(conn), b, revoked, metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer test-only-token"))
}
func resume(cursor uint64) *pb.RunnerFrame {
	return &pb.RunnerFrame{Body: &pb.RunnerFrame_Resume{Resume: &pb.Resume{CommittedCursor: cursor}}}
}
func event(cursor uint64) *pb.ServerFrame {
	return &pb.ServerFrame{EventId: "event", Cursor: cursor, Body: &pb.ServerFrame_Assignment{Assignment: &pb.Assignment{AssignmentId: "work", AttemptEpoch: 1}}}
}
func TestAuthenticatedTLSUnaryTrailersAndSafeErrors(t *testing.T) {
	client, _, _, ctx := setup(t, pb.Role_ROLE_EXECUTOR)
	var trailer metadata.MD
	response, err := client.Presence(ctx, &pb.PresenceRequest{Provider: &pb.ProviderSettings{Client: "copilot"}, Availability: pb.Availability_AVAILABILITY_FREE}, grpc.Trailer(&trailer))
	if err != nil || response.GetRunnerId() != "runner" || len(trailer.Get("bot-space-control")) != 1 {
		t.Fatal("TLS unary/trailer failed", err)
	}
	_, err = client.Inspect(ctx, &pb.InspectRequest{ResourceId: "private"})
	if status.Code(err) != codes.PermissionDenied || strings.Contains(err.Error(), "sentinel") {
		t.Fatal("unsafe error", err)
	}
	unauth := metadata.NewOutgoingContext(ctx, metadata.MD{})
	_, err = client.Inspect(unauth, &pb.InspectRequest{ResourceId: "work"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatal("unauthenticated request admitted", err)
	}
}
func TestBidirectionalIndependentDeliveryAndResume(t *testing.T) {
	client, b, _, ctx := setup(t, pb.Role_ROLE_EXECUTOR)
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = stream.Send(resume(12)); err != nil {
		t.Fatal(err)
	}
	b.events <- event(13)
	delivered, err := stream.Recv()
	if err != nil || delivered.GetCursor() != 13 {
		t.Fatal("server could not send independently", err)
	}
	if cursor := <-b.cursors; cursor != 12 {
		t.Fatal("resume cursor lost")
	}
	frame := &pb.RunnerFrame{RequestId: "ack-request", Body: &pb.RunnerFrame_Ack{Ack: &pb.Ack{Cursor: 13, EventId: "event"}}}
	if err = stream.Send(frame); err != nil {
		t.Fatal(err)
	}
	select {
	case applied := <-b.applied:
		if applied.GetAck().GetCursor() != 13 {
			t.Fatal("ACK lost")
		}
	case <-ctx.Done():
		t.Fatal("receiver blocked by idle sender")
	}
	stream.CloseSend()
	_, err = stream.Recv()
	if err != nil && status.Code(err) != codes.OK && !strings.Contains(err.Error(), "EOF") {
		t.Fatal(err)
	}
	next, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = next.Send(resume(13)); err != nil {
		t.Fatal(err)
	}
	b.events <- event(14)
	delivered, err = next.Recv()
	if err != nil || delivered.GetCursor() != 14 {
		t.Fatal("reconnect failed", err)
	}
}
func TestOpenStreamRevocationAndRoleChecks(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "role", true: "revocation"}[revoke], func(t *testing.T) {
			client, b, revoked, ctx := setup(t, pb.Role_ROLE_EXECUTOR)
			stream, err := client.Connect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			stream.Send(resume(0))
			b.events <- event(1)
			if _, err = stream.Recv(); err != nil {
				t.Fatal(err)
			}
			revoked.Store(revoke)
			stream.Send(&pb.RunnerFrame{RequestId: "vote", Body: &pb.RunnerFrame_Vote{Vote: &pb.Vote{DecisionId: "decision", Round: 1, Choice: "approve"}}})
			_, err = stream.Recv()
			expected := codes.PermissionDenied
			if revoke {
				expected = codes.Unauthenticated
			}
			if status.Code(err) != expected {
				t.Fatalf("got %v want %v", err, expected)
			}
			select {
			case <-b.applied:
				t.Fatal("rejected vote reached backend")
			default:
			}
		})
	}
}
func TestOversizedUnaryRejected(t *testing.T) {
	client, _, _, ctx := setup(t, pb.Role_ROLE_EXECUTOR)
	_, err := client.Presence(ctx, &pb.PresenceRequest{Provider: &pb.ProviderSettings{Client: strings.Repeat("x", MaxMessageBytes+1)}, Availability: pb.Availability_AVAILABILITY_FREE})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal("oversized frame admitted", err)
	}
}

func TestOutgoingAuthorityAndEventValidation(t *testing.T) {
	for _, mode := range []string{"revoked", "stale-cursor", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			client, b, revoked, ctx := setup(t, pb.Role_ROLE_EXECUTOR)
			stream, err := client.Connect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			stream.Send(resume(10))
			select {
			case <-b.cursors:
			case <-ctx.Done():
				t.Fatal("stream did not open")
			}
			e := event(11)
			expected := codes.Internal
			switch mode {
			case "revoked":
				revoked.Store(true)
				expected = codes.Unauthenticated
			case "stale-cursor":
				e.Cursor = 10
			case "oversized":
				e.GetAssignment().Instruction = strings.Repeat("x", MaxMessageBytes+1)
			}
			b.events <- e
			_, err = stream.Recv()
			if status.Code(err) != expected {
				t.Fatalf("invalid outgoing event: %v", err)
			}
		})
	}
}
func TestStreamRequiresResumeAndRejectsOversizedInput(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(map[bool]string{false: "resume", true: "size"}[oversized], func(t *testing.T) {
			client, b, _, ctx := setup(t, pb.Role_ROLE_EXECUTOR)
			stream, err := client.Connect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			expected := codes.InvalidArgument
			if oversized {
				stream.Send(resume(0))
				b.events <- event(1)
				if _, err = stream.Recv(); err != nil {
					t.Fatal(err)
				}
				expected = codes.ResourceExhausted
			}
			text := "progress"
			if oversized {
				text = strings.Repeat("x", MaxMessageBytes+1)
			}
			stream.Send(&pb.RunnerFrame{RequestId: "progress", Body: &pb.RunnerFrame_Progress{Progress: &pb.Progress{Description: text}}})
			_, err = stream.Recv()
			if status.Code(err) != expected {
				t.Fatal("invalid frame accepted", err)
			}
		})
	}
}
