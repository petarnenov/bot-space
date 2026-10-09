//go:build darwin || linux

package runneridentity

import (
	"fmt"
	"path/filepath"
	"testing"

	pb "github.com/petarnenov/bot-space/api/control/v1"
)

func TestDeliveryPersistsBeforeAckAndReplaysPendingAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "executor")
	state, err := OpenState(path, "https://example.com", Executor)
	if err != nil {
		t.Fatal(err)
	}
	project := "00000000-0000-4000-8000-000000000001"
	frame := &pb.ServerFrame{EventId: "00000000-0000-4000-8000-000000000002", Cursor: 1, Body: &pb.ServerFrame_Receipt{Receipt: &pb.Receipt{RequestId: "request-1"}}}
	ack, err := state.PersistDelivery(project, frame)
	if err != nil || ack.Cursor != 1 || ack.EventId != frame.EventId {
		t.Fatal("durable ACK missing", err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = OpenState(path, "https://example.com", Executor)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	pending, err := state.PendingDeliveries(project)
	if err != nil || len(pending) != 1 || pending[0].EventId != frame.EventId {
		t.Fatal("ACKed pending event lost on restart", err)
	}
	if _, err = state.PersistDelivery(project, frame); err != nil {
		t.Fatal("replayed event not deduplicated", err)
	}
	changed := &pb.ServerFrame{EventId: frame.EventId, Cursor: 1, Body: &pb.ServerFrame_Receipt{Receipt: &pb.Receipt{RequestId: "changed"}}}
	if _, err = state.PersistDelivery(project, changed); err == nil {
		t.Fatal("same sequence changed payload")
	}
	skipped := &pb.ServerFrame{EventId: "00000000-0000-4000-8000-000000000003", Cursor: 3, Body: frame.Body}
	if _, err = state.PersistDelivery(project, skipped); err == nil {
		t.Fatal("skipped sequence persisted")
	}
	if err = state.MarkDeliveryApplied(project, 1, frame.EventId); err != nil {
		t.Fatal(err)
	}
	pending, err = state.PendingDeliveries(project)
	if err != nil || len(pending) != 0 {
		t.Fatal("applied event still pending", err)
	}
	if _, err = state.PersistDelivery(project, frame); err != nil {
		t.Fatal("compacted event lost replay fingerprint", err)
	}
	foreign, err := state.PendingDeliveries("00000000-0000-4000-8000-000000000004")
	if err != nil || len(foreign) != 0 {
		t.Fatal("project journals mixed", err)
	}
}

func TestDeliveryBackpressureNeverAcknowledgesUnpersistedData(t *testing.T) {
	state, err := OpenState(filepath.Join(t.TempDir(), "executor"), "https://example.com", Executor)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	project := "00000000-0000-4000-8000-000000000001"
	for sequence := uint64(1); sequence <= pendingLimit; sequence++ {
		frame := &pb.ServerFrame{EventId: fmt.Sprintf("00000000-0000-4000-8000-%012d", sequence), Cursor: sequence, Body: &pb.ServerFrame_Receipt{Receipt: &pb.Receipt{RequestId: "pending"}}}
		if _, err = state.PersistDelivery(project, frame); err != nil {
			t.Fatal(err)
		}
	}
	next := &pb.ServerFrame{EventId: "00000000-0000-4000-8000-000000000017", Cursor: 17, Body: &pb.ServerFrame_Receipt{Receipt: &pb.Receipt{RequestId: "blocked"}}}
	if ack, err := state.PersistDelivery(project, next); err == nil || ack != nil {
		t.Fatal("full inbox acknowledged unpersisted event")
	}
	if err = state.MarkDeliveryApplied(project, 1, "00000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	if _, err = state.PersistDelivery(project, next); err != nil {
		t.Fatal("backpressure did not recover", err)
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	if ack, err := state.PersistDelivery(project, next); err == nil || ack != nil {
		t.Fatal("closed journal acknowledged unpersisted event")
	}
}
