package runneridentity

import (
	"context"
	"strconv"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/control"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IdentityBackend serves authenticated self-inspection until the durable work
// backend is installed. It never advertises free capacity or accepts work.
type IdentityBackend struct{}

func (IdentityBackend) Presence(context.Context, control.Principal, *pb.PresenceRequest) error {
	return status.Error(codes.FailedPrecondition, "execution runtime unavailable")
}
func (IdentityBackend) Inspect(_ context.Context, p control.Principal, r *pb.InspectRequest) (*pb.InspectResponse, error) {
	if r.GetResourceId() != p.RunnerID {
		return nil, status.Error(codes.PermissionDenied, "own identity required")
	}
	return &pb.InspectResponse{ResourceId: p.RunnerID, State: "enrolled", Revision: strconv.FormatUint(p.CredentialEpoch, 10)}, nil
}
func (IdentityBackend) Apply(context.Context, control.Principal, *pb.RunnerFrame) error {
	return status.Error(codes.FailedPrecondition, "execution runtime unavailable")
}
func (IdentityBackend) Pull(ctx context.Context, _ control.Principal, _ uint64) (*pb.ServerFrame, error) {
	// No synthetic events or implicit acceptance while durable delivery is absent.
	return nil, status.Error(codes.FailedPrecondition, "execution runtime unavailable")
}
