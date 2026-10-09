// Package council implements deterministic voting rules. Authorization and
// transactional persistence belong to the control service, not this engine.
package council

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

const MaxRounds = 3

type Choice string

const (
	Approve         Choice = "approve"
	Object          Choice = "object"
	NeedInformation Choice = "need_information"
)

type Status string

const (
	Discussing    Status = "discussing"
	NeedsRevision Status = "needs_revision"
	Accepted      Status = "accepted"
	Blocked       Status = "blocked_no_majority"
)

var (
	ErrInvalid      = errors.New("invalid council input")
	ErrMember       = errors.New("not a council member")
	ErrStale        = errors.New("stale proposal or round")
	ErrVoteConflict = errors.New("member already voted differently")
	ErrState        = errors.New("decision transition not allowed")
	ErrUnchanged    = errors.New("no material change for reconsideration")
)

// Material identifies trusted immutable source revisions. The service must
// verify these against stored records; client labels cannot authorize resets.
type Material struct {
	RootRevision    string
	SpecDigest      string
	EvidenceDigest  string
	CouncilRevision string
}

// Action is an operational proposal. Rationale is explanatory only and does
// not count as materially changed input for resetting exhausted discussion.
type Action struct{ Kind, Target, Value string }
type Proposal struct {
	Actions   []Action
	Rationale string
}
type Vote struct {
	Member string
	Choice Choice
}
type Round struct {
	Number   int
	Proposal Proposal
	Hash     string
	Votes    []Vote
	Status   Status
}
type Snapshot struct {
	ID, Kind, PreviousID, MaterialHash string
	Members                            []string
	Required                           int
	Status                             Status
	Rounds                             []Round
}
type Decision struct {
	snapshot Snapshot
	material Material
}

// Material returns immutable source references for persistence and replacement
// checks. The server must resolve these references from authoritative records.
func (d *Decision) Material() Material { return d.material }

func digest(value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func cloneProposal(p Proposal) Proposal { p.Actions = slices.Clone(p.Actions); return p }
func validProposal(p Proposal) bool {
	if len(p.Actions) == 0 || len(p.Actions) > 64 || len(p.Rationale) > 16384 {
		return false
	}
	for _, a := range p.Actions {
		if strings.TrimSpace(a.Kind) == "" || len(a.Kind) > 64 || len(a.Target) > 1024 || len(a.Value) > 65536 {
			return false
		}
	}
	return true
}
func New(id, kind string, members []string, material Material, proposal Proposal) (*Decision, error) {
	switch kind {
	case "plan", "allocation", "answer", "review", "retry", "integration":
	default:
		return nil, ErrInvalid
	}
	if id == "" || len(members) == 0 || len(members) > 128 || material.RootRevision == "" || material.CouncilRevision == "" || !validProposal(proposal) {
		return nil, ErrInvalid
	}
	members = slices.Clone(members)
	slices.Sort(members)
	for i, m := range members {
		if strings.TrimSpace(m) == "" || (i > 0 && m == members[i-1]) {
			return nil, ErrInvalid
		}
	}
	d := &Decision{material: material, snapshot: Snapshot{ID: id, Kind: kind, MaterialHash: digest(material), Members: members, Required: len(members)/2 + 1, Status: Discussing}}
	d.appendRound(proposal)
	return d, nil
}
func (d *Decision) appendRound(p Proposal) {
	p = cloneProposal(p)
	d.snapshot.Rounds = append(d.snapshot.Rounds, Round{Number: len(d.snapshot.Rounds) + 1, Proposal: p, Hash: digest(p), Status: Discussing})
	d.snapshot.Status = Discussing
}

// Snapshot returns defensive copies; callers cannot alter council membership,
// proposal content or counted evidence through returned slices.
func (d *Decision) Snapshot() Snapshot {
	s := d.snapshot
	s.Members = slices.Clone(s.Members)
	s.Rounds = slices.Clone(s.Rounds)
	for i := range s.Rounds {
		s.Rounds[i].Proposal = cloneProposal(s.Rounds[i].Proposal)
		s.Rounds[i].Votes = slices.Clone(s.Rounds[i].Votes)
	}
	return s
}

// Cast requires an authenticated member identity supplied by the service.
// Identical retries are idempotent even after acceptance; conflicting second
// votes are rejected. Neither time nor offline presence is an input.
func (d *Decision) Cast(member string, round int, hash string, choice Choice) error {
	if !slices.Contains(d.snapshot.Members, member) {
		return ErrMember
	}
	r := &d.snapshot.Rounds[len(d.snapshot.Rounds)-1]
	if r.Number != round || r.Hash != hash {
		return ErrStale
	}
	if choice != Approve && choice != Object && choice != NeedInformation {
		return ErrInvalid
	}
	for _, v := range r.Votes {
		if v.Member == member {
			if v.Choice == choice {
				return nil
			}
			return ErrVoteConflict
		}
	}
	if d.snapshot.Status != Discussing {
		return ErrState
	}
	r.Votes = append(r.Votes, Vote{member, choice})
	approvals := 0
	for _, v := range r.Votes {
		if v.Choice == Approve {
			approvals++
		}
	}
	if approvals >= d.snapshot.Required {
		r.Status = Accepted
		d.snapshot.Status = Accepted
	} else if len(r.Votes) == len(d.snapshot.Members) {
		r.Status = NeedsRevision
		d.snapshot.Status = NeedsRevision
		if r.Number == MaxRounds {
			r.Status = Blocked
			d.snapshot.Status = Blocked
		}
	}
	return nil
}
func (d *Decision) Revise(proposal Proposal) error {
	if d.snapshot.Status != NeedsRevision || len(d.snapshot.Rounds) >= MaxRounds {
		return ErrState
	}
	if !validProposal(proposal) {
		return ErrInvalid
	}
	d.appendRound(proposal)
	return nil
}

// Reconsider preserves the exhausted decision. A rationale-only rewrite,
// reordered member list or new ID cannot reset the cap. Changed actions or
// trusted immutable material can create a linked decision. The server must
// additionally validate the reason for an explicit membership revision.
func (d *Decision) Reconsider(id string, members []string, material Material, p Proposal) (*Decision, error) {
	if d.snapshot.Status != Blocked || id == d.snapshot.ID {
		return nil, ErrState
	}
	n, err := New(id, d.snapshot.Kind, members, material, p)
	if err != nil {
		return nil, err
	}
	changedMembers := !slices.Equal(n.snapshot.Members, d.snapshot.Members)
	if changedMembers && material.CouncilRevision == d.material.CouncilRevision {
		return nil, ErrInvalid
	}
	if n.snapshot.MaterialHash == d.snapshot.MaterialHash {
		for _, previous := range d.snapshot.Rounds {
			if digest(p.Actions) == digest(previous.Proposal.Actions) {
				return nil, ErrUnchanged
			}
		}
	}
	n.snapshot.PreviousID = d.snapshot.ID
	return n, nil
}
