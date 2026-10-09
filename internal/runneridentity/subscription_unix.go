//go:build darwin || linux

package runneridentity

import (
	"context"
	"sync"
	"time"

	pb "github.com/petarnenov/bot-space/api/control/v1"
)

type Subscription struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func (s *Subscription) Close() error { s.once.Do(s.cancel); <-s.done; return nil }

// Subscribe persists before ACK and reconnects from the durable server cursor.
// Pending events remain available for the role dispatcher; reception never
// starts a coding session or repeats an external side effect.
func Subscribe(ctx context.Context, state *State, project string, client pb.ControlClient) *Subscription {
	ctx, cancel := context.WithCancel(ctx)
	s := &Subscription{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		for ctx.Err() == nil {
			receive(ctx, state, project, client)
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return s
}
func receive(ctx context.Context, state *State, project string, client pb.ControlClient) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := client.Connect(streamCtx)
	if err != nil {
		return err
	}
	// Zero is always valid: the server chooses its ACK position, while the local
	// journal deduplicates data persisted before a lost acknowledgement.
	if err = stream.Send(&pb.RunnerFrame{Body: &pb.RunnerFrame_Resume{Resume: &pb.Resume{CommittedCursor: 0}}}); err != nil {
		return err
	}
	for {
		frame, e := stream.Recv()
		if e != nil {
			return e
		}
		ack, e := state.PersistDelivery(project, frame)
		if e != nil {
			return e
		}
		if e = stream.Send(&pb.RunnerFrame{RequestId: frame.EventId, Body: &pb.RunnerFrame_Ack{Ack: ack}}); e != nil {
			return e
		}
	}
}
