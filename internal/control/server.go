// Package control provides native gRPC transport around authenticated durable
// orchestration services. It does not implement storage or GitHub enrollment.
package control

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const MaxMessageBytes = 256 << 10
const OperationTimeout = 5 * time.Second

type Principal struct {
	RunnerID, ProjectID string
	Role                pb.Role
	CredentialEpoch     uint64
}

// Authenticate must check activation, current collaborator authority and role
// every time, including calls on an already-open stream. Errors must be safe.
type Authenticate func(context.Context, string) (Principal, error)

// Backend operations must validate project, resource revisions and role. Pull
// returns durable events after cursor, blocking until new data or cancellation.
// Transmission never commits the receiver's durable acknowledgement.
type Backend interface {
	Presence(context.Context, Principal, *pb.PresenceRequest) error
	Inspect(context.Context, Principal, *pb.InspectRequest) (*pb.InspectResponse, error)
	Apply(context.Context, Principal, *pb.RunnerFrame) error
	Pull(context.Context, Principal, uint64) (*pb.ServerFrame, error)
}
type ResumeValidator interface {
	ValidateResume(context.Context, Principal, uint64) error
}
type Server struct {
	pb.UnimplementedControlServer
	Auth    Authenticate
	Backend Backend
}

func New(auth Authenticate, backend Backend, options ...grpc.ServerOption) (*grpc.Server, error) {
	if auth == nil || backend == nil {
		return nil, errors.New("control dependencies unavailable")
	}
	options = append(options, grpc.MaxRecvMsgSize(MaxMessageBytes), grpc.MaxSendMsgSize(MaxMessageBytes), grpc.MaxConcurrentStreams(32))
	s := grpc.NewServer(options...)
	pb.RegisterControlServer(s, &Server{Auth: auth, Backend: backend})
	return s, nil
}
func (s *Server) principal(ctx context.Context) (Principal, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || len(values[0]) > 4096 {
		return Principal{}, status.Error(codes.Unauthenticated, "runner authentication required")
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return Principal{}, status.Error(codes.Unauthenticated, "runner authentication required")
	}
	check, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	p, err := s.Auth(check, token)
	if err != nil || p.RunnerID == "" || p.ProjectID == "" || (p.Role != pb.Role_ROLE_ARCHITECT && p.Role != pb.Role_ROLE_EXECUTOR) {
		return Principal{}, status.Error(codes.Unauthenticated, "runner authority unavailable")
	}
	return p, nil
}
func safeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "control operation cancelled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "control operation unavailable")
	}
	code := status.Code(err)
	switch code {
	case codes.InvalidArgument, codes.PermissionDenied, codes.NotFound, codes.AlreadyExists, codes.FailedPrecondition, codes.Aborted, codes.ResourceExhausted, codes.Unauthenticated, codes.Unavailable:
		return status.Error(code, "control operation rejected")
	}
	return status.Error(codes.Internal, "control operation failed")
}
func (s *Server) Presence(ctx context.Context, r *pb.PresenceRequest) (*pb.PresenceResponse, error) {
	p, err := s.principal(ctx)
	if err != nil {
		return nil, err
	}
	if r.GetProvider().GetClient() == "" || r.GetAvailability() < pb.Availability_AVAILABILITY_FREE || r.GetAvailability() > pb.Availability_AVAILABILITY_DRAINING || len(r.GetTools()) > 64 || len(r.GetProjectMappings()) > 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid runner presence")
	}
	operation, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	if err = s.Backend.Presence(operation, p, r); err != nil {
		return nil, safeError(err)
	}
	grpc.SetTrailer(ctx, metadata.Pairs("bot-space-control", "v1"))
	return &pb.PresenceResponse{RunnerId: p.RunnerID, ProjectId: p.ProjectID, Role: p.Role, CredentialEpoch: p.CredentialEpoch}, nil
}
func (s *Server) Inspect(ctx context.Context, r *pb.InspectRequest) (*pb.InspectResponse, error) {
	p, err := s.principal(ctx)
	if err != nil {
		return nil, err
	}
	if r.GetResourceId() == "" || len(r.GetResourceId()) > 128 {
		return nil, status.Error(codes.InvalidArgument, "invalid resource reference")
	}
	operation, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	out, err := s.Backend.Inspect(operation, p, r)
	grpc.SetTrailer(ctx, metadata.Pairs("bot-space-control", "v1"))
	return out, safeError(err)
}
func sameIdentity(a, b Principal) bool { return a == b }
func (s *Server) Connect(stream grpc.BidiStreamingServer[pb.RunnerFrame, pb.ServerFrame]) error {
	initial, err := s.principal(stream.Context())
	if err != nil {
		return err
	}
	type received struct {
		frame *pb.RunnerFrame
		err   error
	}
	opening := make(chan received, 1)
	go func() { frame, e := stream.Recv(); opening <- received{frame, e} }()
	timer := time.NewTimer(OperationTimeout)
	defer timer.Stop()
	var first *pb.RunnerFrame
	select {
	case r := <-opening:
		if r.err != nil {
			return safeError(r.err)
		}
		first = r.frame
	case <-timer.C:
		return status.Error(codes.DeadlineExceeded, "control handshake unavailable")
	case <-stream.Context().Done():
		return safeError(stream.Context().Err())
	}
	resume, ok := first.GetBody().(*pb.RunnerFrame_Resume)
	if !ok || resume.Resume == nil {
		return status.Error(codes.InvalidArgument, "resume frame required")
	}
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	if validator, ok := s.Backend.(ResumeValidator); ok {
		operation, stop := context.WithTimeout(ctx, OperationTimeout)
		err = validator.ValidateResume(operation, initial, resume.Resume.GetCommittedCursor())
		stop()
		if err != nil {
			return safeError(err)
		}
	}
	done := make(chan error, 2)
	// Exactly one receiver and sender, with no unbounded transport queue.
	go func() {
		for {
			frame, e := stream.Recv()
			if e != nil {
				if e == io.EOF {
					done <- nil
				} else {
					done <- safeError(e)
				}
				return
			}
			p, e := s.principal(ctx)
			if e != nil {
				done <- e
				return
			}
			if !sameIdentity(initial, p) {
				done <- status.Error(codes.Unauthenticated, "runner identity changed")
				return
			}
			if frame.GetBody() == nil || frame.GetResume() != nil || frame.GetRequestId() == "" || len(frame.GetRequestId()) > 128 {
				done <- status.Error(codes.InvalidArgument, "invalid control frame")
				return
			}
			if initial.Role == pb.Role_ROLE_EXECUTOR && (frame.GetVote() != nil || frame.GetProposal() != nil) {
				done <- status.Error(codes.PermissionDenied, "architect role required")
				return
			}
			if initial.Role == pb.Role_ROLE_ARCHITECT && (frame.GetQuestion() != nil || frame.GetResult() != nil || frame.GetProgress() != nil) {
				done <- status.Error(codes.PermissionDenied, "executor role required")
				return
			}
			operation, stop := context.WithTimeout(ctx, OperationTimeout)
			e = s.Backend.Apply(operation, p, frame)
			stop()
			if e != nil {
				done <- safeError(e)
				return
			}
		}
	}()
	go func() {
		cursor := resume.Resume.GetCommittedCursor()
		for {
			event, e := s.Backend.Pull(ctx, initial, cursor)
			if e != nil {
				done <- safeError(e)
				return
			}
			p, e := s.principal(ctx)
			if e != nil {
				done <- e
				return
			}
			if !sameIdentity(initial, p) {
				done <- status.Error(codes.Unauthenticated, "runner identity changed")
				return
			}
			if event == nil || event.GetBody() == nil || event.GetEventId() == "" || event.GetCursor() <= cursor || proto.Size(event) > MaxMessageBytes {
				done <- status.Error(codes.Internal, "invalid durable control event")
				return
			}
			if e = stream.Send(event); e != nil {
				done <- safeError(e)
				return
			}
			cursor = event.GetCursor()
		}
	}()
	select {
	case <-ctx.Done():
		return safeError(ctx.Err())
	case e := <-done:
		return e
	}
}
