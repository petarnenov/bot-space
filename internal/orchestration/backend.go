// Package orchestration connects durable domain operations to native control.
package orchestration

import (
	"context"
	"errors"
	"strings"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/control"
	"github.com/petarnenov/bot-space/internal/controlevents"
	"github.com/petarnenov/bot-space/internal/council"
	"github.com/petarnenov/bot-space/internal/councilstore"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Backend struct {
	Identities *runneridentity.Store
	Events     *controlevents.Store
	Council    *councilstore.Store
}

func (b *Backend) token(ctx context.Context, p control.Principal) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if b.Identities == nil || len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", status.Error(codes.Unauthenticated, "runner required")
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	actual, err := b.Identities.Authenticate(ctx, token)
	if err != nil || actual != p {
		return "", status.Error(codes.Unauthenticated, "runner changed")
	}
	return token, nil
}
func translated(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, controlevents.ErrForbidden), errors.Is(err, councilstore.ErrForbidden):
		return status.Error(codes.PermissionDenied, "project authority required")
	case errors.Is(err, controlevents.ErrInvalid), errors.Is(err, council.ErrInvalid):
		return status.Error(codes.InvalidArgument, "invalid control operation")
	case errors.Is(err, controlevents.ErrConflict), errors.Is(err, council.ErrVoteConflict):
		return status.Error(codes.AlreadyExists, "conflicting control operation")
	case errors.Is(err, council.ErrMember):
		return status.Error(codes.PermissionDenied, "council member required")
	case errors.Is(err, council.ErrStale), errors.Is(err, council.ErrState), errors.Is(err, councilstore.ErrStale):
		return status.Error(codes.FailedPrecondition, "stale control operation")
	default:
		return status.Error(codes.Unavailable, "control storage unavailable")
	}
}
func (b *Backend) Presence(context.Context, control.Principal, *pb.PresenceRequest) error {
	return status.Error(codes.FailedPrecondition, "provider execution runtime unavailable")
}
func (b *Backend) Inspect(ctx context.Context, p control.Principal, r *pb.InspectRequest) (*pb.InspectResponse, error) {
	return (runneridentity.IdentityBackend{}).Inspect(ctx, p, r)
}
func (b *Backend) ValidateResume(ctx context.Context, p control.Principal, cursor uint64) error {
	token, err := b.token(ctx, p)
	if err != nil {
		return err
	}
	return translated(b.Events.ValidateResume(ctx, token, cursor))
}
func (b *Backend) Apply(ctx context.Context, p control.Principal, frame *pb.RunnerFrame) error {
	token, err := b.token(ctx, p)
	if err != nil {
		return err
	}
	if ack := frame.GetAck(); ack != nil {
		return translated(b.Events.Acknowledge(ctx, token, ack.Cursor, ack.EventId))
	}
	if vote := frame.GetVote(); vote != nil {
		if p.Role != pb.Role_ROLE_ARCHITECT {
			return status.Error(codes.PermissionDenied, "architect required")
		}
		_, err = b.Council.Vote(ctx, token, vote.DecisionId, int(vote.Round), vote.ProposalHash, council.Choice(vote.Choice))
		return translated(err)
	}
	return status.Error(codes.FailedPrecondition, "operation runtime unavailable")
}
func (b *Backend) Pull(ctx context.Context, p control.Principal, cursor uint64) (*pb.ServerFrame, error) {
	token, err := b.token(ctx, p)
	if err != nil {
		return nil, err
	}
	frame, err := b.Events.Next(ctx, token, cursor)
	return frame, translated(err)
}
