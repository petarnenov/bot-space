package council

import (
	"errors"
	"fmt"
	"testing"
)

func fixture(t *testing.T, n int) *Decision {
	t.Helper()
	members := make([]string, n)
	for i := range members {
		members[i] = fmt.Sprintf("architect-%d", i)
	}
	d, err := New("decision-1", "allocation", members, Material{RootRevision: "root:1", SpecDigest: "spec:1", CouncilRevision: "council:1"}, Proposal{Actions: []Action{{Kind: "assign", Target: "executor-1", Value: "work:1"}}, Rationale: "initial"})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func cast(t *testing.T, d *Decision, member int, choice Choice) {
	t.Helper()
	s := d.Snapshot()
	r := s.Rounds[len(s.Rounds)-1]
	if err := d.Cast(fmt.Sprintf("architect-%d", member), r.Number, r.Hash, choice); err != nil {
		t.Fatal(err)
	}
}
func TestStrictMajorityFixedCouncil(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 6} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			d := fixture(t, n)
			required := n/2 + 1
			if d.Snapshot().Required != required {
				t.Fatal("incorrect threshold")
			}
			for i := 0; i < required-1; i++ {
				cast(t, d, i, Approve)
			}
			if d.Snapshot().Status != Discussing {
				t.Fatal("accepted without majority")
			}
			cast(t, d, required-1, Approve)
			if d.Snapshot().Status != Accepted {
				t.Fatal("majority not accepted")
			}
		})
	}
}
func TestTieContinuesUntilThreeRoundsAndMaterialReconsideration(t *testing.T) {
	d := fixture(t, 4)
	for round := 1; round <= 3; round++ {
		cast(t, d, 0, Approve)
		cast(t, d, 1, Approve)
		cast(t, d, 2, Object)
		if d.Snapshot().Status != Discussing {
			t.Fatal("missing vote closed round")
		}
		cast(t, d, 3, NeedInformation)
		s := d.Snapshot()
		if round < 3 {
			if s.Status != NeedsRevision {
				t.Fatal("tie accepted")
			}
			p := s.Rounds[round-1].Proposal
			p.Rationale = fmt.Sprint("discussion ", round)
			if err := d.Revise(p); err != nil {
				t.Fatal(err)
			}
		} else if s.Status != Blocked {
			t.Fatal("third tie not blocked")
		}
	}
	s := d.Snapshot()
	p := s.Rounds[2].Proposal
	if !errors.Is(d.Revise(p), ErrState) {
		t.Fatal("fourth round allowed")
	}
	p.Rationale = "cosmetic restart"
	if _, err := d.Reconsider("decision-2", s.Members, d.material, p); !errors.Is(err, ErrUnchanged) {
		t.Fatal("rationale reset exhausted rounds", err)
	}
	reordered := []string{s.Members[3], s.Members[2], s.Members[1], s.Members[0]}
	if _, err := d.Reconsider("decision-2", reordered, d.material, p); !errors.Is(err, ErrUnchanged) {
		t.Fatal("member ordering reset cap", err)
	}
	if _, err := d.Reconsider("decision-2", s.Members[:3], d.material, p); !errors.Is(err, ErrInvalid) {
		t.Fatal("offline member silently removed", err)
	}
	material := d.material
	material.EvidenceDigest = "new-check-evidence"
	n, err := d.Reconsider("decision-2", s.Members, material, p)
	if err != nil {
		t.Fatal(err)
	}
	if n.Snapshot().PreviousID != s.ID || len(n.Snapshot().Rounds) != 1 || d.Snapshot().Status != Blocked {
		t.Fatal("reconsideration lost provenance")
	}
	p.Actions[0].Target = "executor-2"
	if _, err := d.Reconsider("decision-3", s.Members, d.material, p); err != nil {
		t.Fatal("material assignment change rejected", err)
	}
}
func TestVoteIdentityRetryAndStaleProposal(t *testing.T) {
	d := fixture(t, 2)
	r := d.Snapshot().Rounds[0]
	if !errors.Is(d.Cast("outsider", 1, r.Hash, Approve), ErrMember) {
		t.Fatal("outsider voted")
	}
	if !errors.Is(d.Cast("architect-0", 1, "old-hash", Approve), ErrStale) {
		t.Fatal("wrong hash voted")
	}
	cast(t, d, 0, Approve)
	cast(t, d, 0, Approve)
	if len(d.Snapshot().Rounds[0].Votes) != 1 {
		t.Fatal("duplicate identity added support")
	}
	if !errors.Is(d.Cast("architect-0", 1, r.Hash, Object), ErrVoteConflict) {
		t.Fatal("conflicting duplicate accepted")
	}
	cast(t, d, 1, Object)
	if err := d.Revise(r.Proposal); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(d.Cast("architect-0", 1, r.Hash, Approve), ErrStale) {
		t.Fatal("prior-round vote replay accepted")
	}
	if len(d.Snapshot().Rounds[1].Votes) != 0 {
		t.Fatal("votes carried into new round")
	}
	cast(t, d, 0, Approve)
	cast(t, d, 1, Approve)
	cast(t, d, 1, Approve)
	if !errors.Is(d.Revise(r.Proposal), ErrState) {
		t.Fatal("accepted decision revised")
	}
}
func TestSnapshotCannotMutateAuthority(t *testing.T) {
	d := fixture(t, 2)
	s := d.Snapshot()
	s.Members[0] = "outsider"
	s.Rounds[0].Proposal.Actions[0].Target = "other"
	cast(t, d, 0, Approve)
	s = d.Snapshot()
	s.Rounds[0].Votes[0].Choice = Object
	fresh := d.Snapshot()
	if fresh.Members[0] != "architect-0" || fresh.Rounds[0].Proposal.Actions[0].Target != "executor-1" || fresh.Rounds[0].Votes[0].Choice != Approve {
		t.Fatal("snapshot mutated authoritative state")
	}
}
func TestInvalidCouncilAndProposal(t *testing.T) {
	p := Proposal{Actions: []Action{{Kind: "plan"}}}
	m := Material{RootRevision: "r", CouncilRevision: "c"}
	for _, members := range [][]string{nil, {"a", "a"}, {" "}} {
		if _, err := New("id", "plan", members, m, p); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid members accepted")
		}
	}
	if _, err := New("id", "human_override", []string{"a"}, m, p); !errors.Is(err, ErrInvalid) {
		t.Fatal("human override kind accepted")
	}
	if _, err := New("id", "plan", []string{"a"}, m, Proposal{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty proposal accepted")
	}
	d := fixture(t, 2)
	r := d.Snapshot().Rounds[0]
	if !errors.Is(d.Cast("architect-0", 1, r.Hash, "abstain"), ErrInvalid) {
		t.Fatal("unknown choice accepted")
	}
	if !errors.Is(d.Revise(p), ErrState) {
		t.Fatal("pending members skipped")
	}
}
