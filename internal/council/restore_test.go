package council

import "testing"

func TestRestoreReplaysAndRejectsForgedAcceptance(t *testing.T) {
	d := fixture(t, 4)
	cast(t, d, 0, Approve)
	cast(t, d, 1, Approve)
	snapshot := d.Snapshot()
	restarted, err := Restore(snapshot, d.material)
	if err != nil || restarted.Snapshot().Status != Discussing {
		t.Fatal("restart lost pending vote evidence", err)
	}
	cast(t, restarted, 2, Approve)
	if restarted.Snapshot().Status != Accepted {
		t.Fatal("restart changed majority threshold")
	}
	for _, corrupt := range []func(*Snapshot){
		func(s *Snapshot) { s.Required = 2 },
		func(s *Snapshot) { s.Status = Accepted },
		func(s *Snapshot) { s.MaterialHash = "changed" },
		func(s *Snapshot) { s.Rounds[0].Hash = "changed" },
		func(s *Snapshot) { s.Rounds[0].Votes = append(s.Rounds[0].Votes, s.Rounds[0].Votes[0]) },
		func(s *Snapshot) { s.Rounds[0].Votes[0].Member = "outsider" },
	} {
		s := d.Snapshot()
		corrupt(&s)
		if _, err := Restore(s, d.material); err == nil {
			t.Fatal("corrupted durable state accepted")
		}
	}
}

func TestRestorePreservesExhaustionAcrossRounds(t *testing.T) {
	d := fixture(t, 2)
	for round := 1; round <= MaxRounds; round++ {
		cast(t, d, 0, Approve)
		cast(t, d, 1, Object)
		if round < MaxRounds {
			p := d.Snapshot().Rounds[round-1].Proposal
			p.Rationale = "Another discussion round"
			if err := d.Revise(p); err != nil {
				t.Fatal(err)
			}
		}
	}
	restarted, err := Restore(d.Snapshot(), d.material)
	if err != nil || restarted.Snapshot().Status != Blocked {
		t.Fatal("restart reset exhausted discussion", err)
	}
	if err := restarted.Revise(d.Snapshot().Rounds[0].Proposal); err == nil {
		t.Fatal("restart allowed fourth round")
	}
}
