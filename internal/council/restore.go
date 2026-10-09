package council

// Restore replays durable votes instead of trusting persisted thresholds or
// acceptance flags. Corrupt, reordered or impossible histories fail closed.
func Restore(snapshot Snapshot, material Material) (*Decision, error) {
	if len(snapshot.Rounds) == 0 || len(snapshot.Rounds) > MaxRounds {
		return nil, ErrInvalid
	}
	d, err := New(snapshot.ID, snapshot.Kind, snapshot.Members, material, snapshot.Rounds[0].Proposal)
	if err != nil {
		return nil, err
	}
	if snapshot.PreviousID == snapshot.ID {
		return nil, ErrInvalid
	}
	d.snapshot.PreviousID = snapshot.PreviousID
	for i, round := range snapshot.Rounds {
		if i > 0 {
			if err = d.Revise(round.Proposal); err != nil {
				return nil, ErrInvalid
			}
		}
		if round.Number != i+1 || round.Hash != d.snapshot.Rounds[i].Hash {
			return nil, ErrInvalid
		}
		for _, vote := range round.Votes {
			if err = d.Cast(vote.Member, round.Number, round.Hash, vote.Choice); err != nil {
				return nil, ErrInvalid
			}
		}
	}
	if digest(d.Snapshot()) != digest(snapshot) {
		return nil, ErrInvalid
	}
	return d, nil
}
