package controlprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestProbeNativeH2CWithLegacyHTTPAndNoTaskMutation(t *testing.T) {
	token := strings.Repeat("x", 32)
	handler, stop, err := Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "legacy") }), token)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "legacy" {
		t.Fatal("legacy route changed")
	}
	conn, err := grpc.NewClient(strings.TrimPrefix(server.URL, "http://"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewControlClient(conn)
	deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx := metadata.NewOutgoingContext(deadline, metadata.Pairs("authorization", "Bearer "+token))
	var trailers metadata.MD
	p, err := client.Presence(ctx, &pb.PresenceRequest{Provider: &pb.ProviderSettings{Client: "probe"}, Availability: pb.Availability_AVAILABILITY_FREE}, grpc.Trailer(&trailers))
	if err != nil || p.GetRunnerId() != "transport-probe" || len(trailers.Get("bot-space-control")) != 1 {
		t.Fatal("h2c unary/trailers failed", err)
	}
	_, err = client.Inspect(deadline, &pb.InspectRequest{ResourceId: "test"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatal("unauthenticated probe admitted", err)
	}
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&pb.RunnerFrame{Body: &pb.RunnerFrame_Resume{Resume: &pb.Resume{CommittedCursor: 20}}})
	event, err := stream.Recv()
	if err != nil || event.GetCursor() != 21 {
		t.Fatal("independent event/resume failed", err)
	}
	stream.Send(&pb.RunnerFrame{RequestId: "ack", Body: &pb.RunnerFrame_Ack{Ack: &pb.Ack{Cursor: 21, EventId: event.EventId}}})
	for {
		result, err := client.Inspect(ctx, &pb.InspectRequest{ResourceId: "nonce"})
		if err != nil {
			t.Fatal(err)
		}
		if result.GetRevision() == "1" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("ACK not processed")
		case <-time.After(10 * time.Millisecond):
		}
	}
	stream.Send(&pb.RunnerFrame{RequestId: "result", Body: &pb.RunnerFrame_Result{Result: &pb.Result{AssignmentId: "must-not-mutate"}}})
	for {
		_, err = stream.Recv()
		if err != nil {
			break
		}
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal("probe accepted task mutation", err)
	}
}

func TestNativeTLSCertificateVerification(t *testing.T) {
	certificateSource := httptest.NewTLSServer(http.NotFoundHandler())
	pair := certificateSource.TLS.Certificates[0]
	certificateSource.Close()
	roots := x509.NewCertPool()
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots.AddCert(cert)
	server, err := New(strings.Repeat("x", 32), grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	go server.Serve(listener)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+strings.Repeat("x", 32)))
	result, err := pb.NewControlClient(conn).Inspect(ctx, &pb.InspectRequest{ResourceId: "verified-tls"})
	if err != nil || result.GetState() != "transport_probe" {
		t.Fatal("verified TLS failed", err)
	}
	wrong, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS12})))
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	_, err = pb.NewControlClient(wrong).Inspect(ctx, &pb.InspectRequest{ResourceId: "untrusted"})
	if status.Code(err) != codes.Unavailable {
		t.Fatal("untrusted certificate accepted", err)
	}
}
