package councilstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/council"
)

// checkEvidence requires the replayable snapshot and immutable normalized
// records to agree. A forged snapshot alone cannot authorize a decision.
func checkEvidence(ctx context.Context, tx pgx.Tx, r Record) error {
	rounds, err := tx.Query(ctx, `SELECT round,proposal_hash,proposal FROM mailbox.council_rounds WHERE project_id=$1 AND decision_id=$2 ORDER BY round`, r.Project, r.Snapshot.ID)
	if err != nil {
		return ErrUnavailable
	}
	count := 0
	for rounds.Next() {
		var number int
		var hash string
		var raw []byte
		var proposal council.Proposal
		if err = rounds.Scan(&number, &hash, &raw); err != nil || json.Unmarshal(raw, &proposal) != nil || count >= len(r.Snapshot.Rounds) {
			rounds.Close()
			return ErrUnavailable
		}
		expected := r.Snapshot.Rounds[count]
		actualJSON, _ := json.Marshal(proposal)
		expectedJSON, _ := json.Marshal(expected.Proposal)
		if number != expected.Number || hash != expected.Hash || string(actualJSON) != string(expectedJSON) {
			rounds.Close()
			return ErrUnavailable
		}
		count++
	}
	err = rounds.Err()
	rounds.Close()
	if err != nil || count != len(r.Snapshot.Rounds) {
		return ErrUnavailable
	}
	expectedVotes := map[string]council.Choice{}
	for _, round := range r.Snapshot.Rounds {
		for _, vote := range round.Votes {
			expectedVotes[fmt.Sprintf("%d:%s", round.Number, vote.Member)] = vote.Choice
		}
	}
	votes, err := tx.Query(ctx, `SELECT round,runner_id::text,choice FROM mailbox.council_votes WHERE project_id=$1 AND decision_id=$2`, r.Project, r.Snapshot.ID)
	if err != nil {
		return ErrUnavailable
	}
	count = 0
	for votes.Next() {
		var round int
		var member string
		var choice council.Choice
		if votes.Scan(&round, &member, &choice) != nil || expectedVotes[fmt.Sprintf("%d:%s", round, member)] != choice {
			votes.Close()
			return ErrUnavailable
		}
		count++
	}
	err = votes.Err()
	votes.Close()
	if err != nil || count != len(expectedVotes) {
		return ErrUnavailable
	}
	var round int
	var hash string
	err = tx.QueryRow(ctx, `SELECT round,proposal_hash FROM mailbox.council_commits WHERE project_id=$1 AND decision_id=$2`, r.Project, r.Snapshot.ID).Scan(&round, &hash)
	if r.Snapshot.Status != council.Accepted {
		if err != pgx.ErrNoRows {
			return ErrUnavailable
		}
		return nil
	}
	last := r.Snapshot.Rounds[len(r.Snapshot.Rounds)-1]
	if err != nil || round != last.Number || hash != last.Hash {
		return ErrUnavailable
	}
	return nil
}
