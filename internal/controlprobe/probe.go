// Package controlprobe provides an explicitly enabled transport diagnostic.
// Its synthetic events never access task, council or identity storage.
package controlprobe

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/control"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type backend struct{ acks atomic.Uint64 }

func (b *backend) Presence(context.Context, control.Principal, *pb.PresenceRequest) error { return nil }
func (b *backend) Inspect(_ context.Context, _ control.Principal, r *pb.InspectRequest) (*pb.InspectResponse, error) {
	if r.ResourceId == "denied" {
		return nil, status.Error(codes.PermissionDenied, "probe denied")
	}
	return &pb.InspectResponse{ResourceId: r.ResourceId, State: "transport_probe", Revision: strconv.FormatUint(b.acks.Load(), 10)}, nil
}
func (b *backend) Apply(_ context.Context, _ control.Principal, r *pb.RunnerFrame) error {
	if r.GetAck() == nil {
		return status.Error(codes.PermissionDenied, "probe accepts acknowledgements only")
	}
	b.acks.Add(1)
	return nil
}
func (b *backend) Pull(ctx context.Context, _ control.Principal, cursor uint64) (*pb.ServerFrame, error) {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	if cursor == ^uint64(0) {
		return nil, status.Error(codes.InvalidArgument, "invalid probe cursor")
	}
	return &pb.ServerFrame{EventId: fmt.Sprintf("transport-probe-%d", cursor+1), Cursor: cursor + 1, Body: &pb.ServerFrame_Receipt{Receipt: &pb.Receipt{RequestId: "transport-probe"}}}, nil
}

// Wrap serves native HTTP/2 gRPC separately from legacy buffered HTTP routes.
// The caller must invoke Stop during shutdown. Empty token leaves routes closed.
func Wrap(legacy http.Handler, token string) (http.Handler, func(), error) {
	if len(token) < 32 || len(token) > 128 || strings.ContainsAny(token, " \t\r\n") {
		return nil, nil, errors.New("invalid control probe configuration")
	}
	auth := func(_ context.Context, presented string) (control.Principal, error) {
		if subtle.ConstantTimeCompare([]byte(token), []byte(presented)) != 1 {
			return control.Principal{}, errors.New("probe authentication denied")
		}
		return control.Principal{RunnerID: "transport-probe", ProjectID: "transport-probe", Role: pb.Role_ROLE_EXECUTOR, CredentialEpoch: 1}, nil
	}
	server, err := control.New(auth, &backend{})
	if err != nil {
		return nil, nil, err
	}
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/botspace.control.v1.Control/") {
			// Bypass legacy request-body buffering and per-response duration bounds.
			// Opening/authentication/mutations remain technically bounded by control.
			rc := http.NewResponseController(w)
			_ = rc.SetReadDeadline(time.Time{})
			_ = rc.SetWriteDeadline(time.Time{})
			server.ServeHTTP(w, r)
			return
		}
		legacy.ServeHTTP(w, r)
	})
	return h2c.NewHandler(mux, &http2.Server{MaxConcurrentStreams: 32, MaxReadFrameSize: 1 << 20}), server.Stop, nil
}
