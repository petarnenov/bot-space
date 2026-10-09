package runneridentity

import (
	"context"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/control"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestIdentityOnlyBackendRejectsWorkAndUnrelatedInspection(t *testing.T) {
	b := IdentityBackend{}
	p := control.Principal{RunnerID: project, ProjectID: project, Role: pb.Role_ROLE_EXECUTOR, CredentialEpoch: 2}
	ctx := context.Background()
	out, err := b.Inspect(ctx, p, &pb.InspectRequest{ResourceId: p.RunnerID})
	if err != nil || out.State != "enrolled" || out.Revision != "2" {
		t.Fatal("own identity unavailable", err)
	}
	if _, err = b.Inspect(ctx, p, &pb.InspectRequest{ResourceId: "unrelated"}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("unrelated identity exposed", err)
	}
	if status.Code(b.Presence(ctx, p, &pb.PresenceRequest{})) != codes.FailedPrecondition {
		t.Fatal("unfinished executor advertised capacity")
	}
	if status.Code(b.Apply(ctx, p, &pb.RunnerFrame{})) != codes.FailedPrecondition {
		t.Fatal("unfinished backend accepted work")
	}
	if _, err = b.Pull(ctx, p, 0); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("unfinished backend fabricated event")
	}
}
