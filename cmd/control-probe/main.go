// control-probe verifies transport only; it never registers operational agents.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Transport probe failed:", err)
		os.Exit(1)
	}
}
func run() error {
	endpoint := flag.String("endpoint", "", "gRPC host:port")
	caFile := flag.String("ca-file", "", "Trusted control CA certificate file")
	local := flag.Bool("local-plaintext", false, "Allow plaintext loopback test only")
	flag.Parse()
	if flag.NArg() != 0 || *endpoint == "" {
		return errors.New("endpoint required")
	}
	token := os.Getenv("CONTROL_PROBE_TOKEN")
	if len(token) < 32 {
		return errors.New("private probe token unavailable")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if *caFile != "" {
		pem, err := os.ReadFile(*caFile)
		if err != nil {
			return errors.New("control CA unavailable")
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return errors.New("invalid control CA")
		}
		tlsConfig.RootCAs = roots
	}
	var creds credentials.TransportCredentials = credentials.NewTLS(tlsConfig)
	if *local {
		host, _, err := net.SplitHostPort(*endpoint)
		ip := net.ParseIP(host)
		if err != nil || !(host == "localhost" || ip != nil && ip.IsLoopback()) {
			return errors.New("plaintext restricted to loopback")
		}
		creds = insecure.NewCredentials()
	}
	conn, err := grpc.NewClient(*endpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}
	defer conn.Close()
	client := pb.NewControlClient(conn)
	deadline, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ctx := metadata.NewOutgoingContext(deadline, metadata.Pairs("authorization", "Bearer "+token))
	_, err = client.Inspect(deadline, &pb.InspectRequest{ResourceId: "probe"})
	if status.Code(err) != codes.Unauthenticated {
		return fmt.Errorf("unauthenticated status: %s", status.Code(err))
	}
	var trailers metadata.MD
	response, err := client.Presence(ctx, &pb.PresenceRequest{Provider: &pb.ProviderSettings{Client: "transport-probe"}, Availability: pb.Availability_AVAILABILITY_FREE}, grpc.Trailer(&trailers))
	if err != nil {
		return fmt.Errorf("unary status: %s", status.Code(err))
	}
	if response.GetRunnerId() != "transport-probe" || strings.Join(trailers.Get("bot-space-control"), ",") != "v1" {
		return errors.New("unary response/trailers mismatch")
	}
	_, err = client.Inspect(ctx, &pb.InspectRequest{ResourceId: "denied"})
	if status.Code(err) != codes.PermissionDenied {
		return errors.New("error trailer status mismatch")
	}
	initial, err := client.Inspect(ctx, &pb.InspectRequest{ResourceId: "probe"})
	if err != nil {
		return err
	}
	before, err := strconv.ParseUint(initial.GetRevision(), 10, 64)
	if err != nil {
		return errors.New("probe revision invalid")
	}
	streamCtx, stop := context.WithCancel(ctx)
	stream, err := client.Connect(streamCtx)
	if err != nil {
		stop()
		return err
	}
	if err = stream.Send(&pb.RunnerFrame{Body: &pb.RunnerFrame_Resume{Resume: &pb.Resume{CommittedCursor: 20}}}); err != nil {
		stop()
		return err
	}
	event, err := stream.Recv()
	if err != nil {
		stop()
		return fmt.Errorf("stream status: %s", status.Code(err))
	}
	if event.GetCursor() != 21 || event.GetReceipt() == nil {
		stop()
		return errors.New("independent stream delivery mismatch")
	}
	if err = stream.Send(&pb.RunnerFrame{RequestId: "transport-ack", Body: &pb.RunnerFrame_Ack{Ack: &pb.Ack{Cursor: event.Cursor, EventId: event.EventId}}}); err != nil {
		stop()
		return err
	}
	for {
		result, e := client.Inspect(ctx, &pb.InspectRequest{ResourceId: "probe"})
		if e != nil {
			stop()
			return e
		}
		count, e := strconv.ParseUint(result.GetRevision(), 10, 64)
		if e != nil {
			stop()
			return e
		}
		if count > before {
			break
		}
		select {
		case <-ctx.Done():
			stop()
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	stop()
	next, err := client.Connect(ctx)
	if err != nil {
		return err
	}
	if err = next.Send(&pb.RunnerFrame{Body: &pb.RunnerFrame_Resume{Resume: &pb.Resume{CommittedCursor: 21}}}); err != nil {
		return err
	}
	event, err = next.Recv()
	if err != nil || event.GetCursor() != 22 {
		return errors.New("stream reconnect mismatch")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"endpoint": *endpoint, "tls": !*local, "unary": true, "status_trailers": true, "bidirectional": true, "reconnect_cursor": true, "scope": "transport-only; synthetic events; no durable-storage proof"})
}
