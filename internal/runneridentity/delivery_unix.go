//go:build darwin || linux

package runneridentity

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/security"
	"google.golang.org/protobuf/proto"
)

const deliveryLimit = 4 << 20
const pendingLimit = 16

type savedEvent struct {
	ID       string
	Sequence uint64
	Hash     string
	Payload  []byte
	Applied  bool
}
type deliveryJournal struct {
	Version int
	Project string
	Cursor  uint64
	Events  []savedEvent
}

func (s *State) delivery(project string) (deliveryJournal, error) {
	out := deliveryJournal{Version: 1, Project: project, Events: []savedEvent{}}
	raw, err := s.read("delivery-"+project+".json", deliveryLimit)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal(raw, &out) != nil || out.Version != 1 || out.Project != project || len(out.Events) > 128 {
		return out, ErrStateUnavailable
	}
	var previous uint64
	for i, event := range out.Events {
		if !security.ValidUUID(event.ID) || event.Sequence < 1 || event.Sequence > out.Cursor || (i > 0 && event.Sequence != previous+1) {
			return out, ErrStateUnavailable
		}
		previous = event.Sequence
		if !event.Applied && len(event.Payload) == 0 {
			return out, ErrStateUnavailable
		}
		if len(event.Payload) > 0 && security.Hash(string(event.Payload)) != event.Hash {
			return out, ErrStateUnavailable
		}
	}
	if len(out.Events) > 0 && previous != out.Cursor {
		return out, ErrStateUnavailable
	}
	if out.Cursor > 0 && len(out.Events) == 0 {
		return out, ErrStateUnavailable
	}
	return out, nil
}
func (s *State) saveDelivery(j deliveryJournal) error {
	raw, err := json.Marshal(j)
	if err != nil || len(raw) > deliveryLimit {
		return ErrStateUnavailable
	}
	return s.write("delivery-"+j.Project+".json", raw)
}

// PersistDelivery returns an ACK only after file and directory fsync succeed.
// Receiving data never itself executes a task or changes a native session.
func (s *State) PersistDelivery(project string, frame *pb.ServerFrame) (*pb.Ack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return nil, ErrStateUnavailable
	}
	project = strings.ToLower(project)
	if !security.ValidUUID(project) || frame == nil || !security.ValidUUID(frame.EventId) || frame.Cursor < 1 || frame.Body == nil || proto.Size(frame) > 132<<10 {
		return nil, ErrInvalid
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(frame)
	if err != nil {
		return nil, ErrInvalid
	}
	j, err := s.delivery(project)
	if err != nil {
		return nil, err
	}
	hash := security.Hash(string(raw))
	if frame.Cursor <= j.Cursor {
		for _, event := range j.Events {
			if event.Sequence == frame.Cursor {
				if event.ID != frame.EventId || event.Hash != hash {
					return nil, ErrInvalid
				}
				return &pb.Ack{Cursor: frame.Cursor, EventId: frame.EventId}, nil
			}
		}
		return nil, ErrInvalid
	}
	if frame.Cursor != j.Cursor+1 {
		return nil, ErrInvalid
	}
	pending := 0
	for _, event := range j.Events {
		if event.ID == frame.EventId {
			return nil, ErrInvalid
		}
		if !event.Applied {
			pending++
		}
	}
	if pending >= pendingLimit {
		return nil, ErrStateUnavailable
	}
	j.Events = append(j.Events, savedEvent{ID: frame.EventId, Sequence: frame.Cursor, Hash: hash, Payload: raw})
	j.Cursor = frame.Cursor
	for len(j.Events) > 128 {
		if !j.Events[0].Applied {
			return nil, ErrStateUnavailable
		}
		j.Events = j.Events[1:]
	}
	if err = s.saveDelivery(j); err != nil {
		return nil, err
	}
	return &pb.Ack{Cursor: frame.Cursor, EventId: frame.EventId}, nil
}

func (s *State) PendingDeliveries(project string) ([]*pb.ServerFrame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return nil, ErrStateUnavailable
	}
	project = strings.ToLower(project)
	if !security.ValidUUID(project) {
		return nil, ErrInvalid
	}
	j, err := s.delivery(project)
	if err != nil {
		return nil, err
	}
	out := []*pb.ServerFrame{}
	for _, event := range j.Events {
		if event.Applied {
			continue
		}
		frame := &pb.ServerFrame{}
		if proto.Unmarshal(event.Payload, frame) != nil || frame.EventId != event.ID || frame.Cursor != event.Sequence || frame.Body == nil {
			return nil, ErrStateUnavailable
		}
		out = append(out, frame)
	}
	return out, nil
}

// MarkDeliveryApplied is used only after the domain transition is durable.
// Retained IDs/hashes deduplicate repeated delivery after payload compaction.
func (s *State) MarkDeliveryApplied(project string, sequence uint64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return ErrStateUnavailable
	}
	project = strings.ToLower(project)
	if !security.ValidUUID(project) {
		return ErrInvalid
	}
	j, err := s.delivery(project)
	if err != nil {
		return err
	}
	for i := range j.Events {
		if j.Events[i].Sequence == sequence && j.Events[i].ID == id {
			j.Events[i].Applied = true
			j.Events[i].Payload = nil
			return s.saveDelivery(j)
		}
	}
	return ErrInvalid
}
