package integration

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/contracts"
	"github.com/petarnenov/bot-space/internal/council"
	"github.com/petarnenov/bot-space/internal/councilstore"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
)

func TestDurableCouncilConcurrentVotesAndFixedOfflineMembership(t *testing.T) {
	ctx, pool, _, workspace, owner := teams(t)
	var project string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, workspace.ID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	sessions := &identity.Sessions{Pool: pool}
	_, human, err := sessions.Login(ctx, owner.GitHubID, owner.Username, "")
	if err != nil {
		t.Fatal(err)
	}
	queue := &backlog.Store{Pool: pool, Sessions: sessions, Authority: &backlogAuthority{true}}
	root, err := queue.Create(ctx, human, backlog.Input{ProjectID: project, Key: "council", Title: "Council plan", Description: "Verified scope"})
	if err != nil {
		t.Fatal(err)
	}
	tokens, ids := []string{}, []string{}
	for i := 0; i < 4; i++ {
		secret, _ := security.Secret()
		token := runneridentity.TokenPrefix + secret
		var id string
		key := security.Hash("council-machine-" + string(rune('a'+i)))
		if err = pool.QueryRow(ctx, `INSERT INTO mailbox.project_runners(project_id,owner_github_id,public_key,role,credential_hash,credential_expires_at) VALUES($1,101,decode($2,'hex'),'architect',$3,clock_timestamp()+interval '15 minutes') RETURNING id::text`, project, key, security.Hash(token)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
		ids = append(ids, id)
	}
	// No heartbeat or live credential is required for retaining a registered vote.
	if _, err = pool.Exec(ctx, `UPDATE mailbox.project_runners SET credential_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, ids[3]); err != nil {
		t.Fatal(err)
	}
	content := contracts.Content{ProjectID: project, RootID: root.ID, RootRevision: 1, RepositoryID: 42, BaseCommit: strings.Repeat("a", 40), Change: "feature", Tasks: []string{"1.1"}, Scenarios: []string{"work::Output::Success"}, Artifacts: map[string]string{}}
	for _, file := range []string{"proposal.md", "design.md", "tasks.md", "specs/work/spec.md"} {
		content.Artifacts["openspec/changes/feature/"+file] = strings.Repeat("b", 64)
	}
	authority := &runnerAuthority{allowed: map[int64]bool{101: true}}
	identities := &runneridentity.Store{Pool: pool, Authority: authority}
	contractStore := &contracts.Store{Pool: pool, Identities: identities}
	contract, err := contractStore.Publish(ctx, tokens[0], content)
	if err != nil {
		t.Fatal(err)
	}
	// Storage fixtures seed trusted validation; actual Git/OpenSpec validation is
	// independently covered by TestContractPublicationAuthorityRevisionAndRestart.
	if _, err = pool.Exec(ctx, `INSERT INTO mailbox.contract_validations(project_id,contract_id,contract_hash,validator_runner_id) VALUES($1,$2,$3,$4)`, project, contract.ID, contract.Hash, ids[0]); err != nil {
		t.Fatal(err)
	}
	store := &councilstore.Store{Pool: pool, Identities: identities}
	proposal := council.Proposal{Actions: []council.Action{{Kind: "plan", Target: contract.ID, Value: "Implement verified scope"}}, Rationale: "Initial plan"}
	type result struct {
		record councilstore.Record
		err    error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() { r, e := store.OpenPlan(ctx, tokens[0], contract.ID, proposal); results <- result{r, e} }()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.record.Snapshot.ID != second.record.Snapshot.ID {
		t.Fatal("coordinators created conflicting heads", first.err, second.err)
	}
	decision := first.record
	id := decision.Snapshot.ID
	hash := decision.Snapshot.Rounds[0].Hash
	if len(decision.Snapshot.Members) != 4 || decision.Snapshot.Required != 3 {
		t.Fatal("offline member shrank majority")
	}
	for i := 0; i < 2; i++ {
		go func(i int) { r, e := store.Vote(ctx, tokens[i], id, 1, hash, council.Approve); results <- result{r, e} }(i)
	}
	for range 2 {
		r := <-results
		if r.err != nil || r.record.Snapshot.Status != council.Discussing {
			t.Fatal("two votes accepted four-member council", r.err)
		}
	}
	restarted := &councilstore.Store{Pool: pool, Identities: identities}
	for range 2 {
		go func() { r, e := restarted.Vote(ctx, tokens[2], id, 1, hash, council.Approve); results <- result{r, e} }()
	}
	for range 2 {
		r := <-results
		if r.err != nil || r.record.Snapshot.Status != council.Accepted {
			t.Fatal("concurrent duplicate majority vote", r.err)
		}
	}
	if _, err = store.Vote(ctx, tokens[0], id, 1, hash, council.Object); !errors.Is(err, council.ErrVoteConflict) {
		t.Fatal("conflicting vote rewritten", err)
	}
	if _, err = store.Vote(ctx, tokens[0], id, 1, strings.Repeat("c", 64), council.Approve); !errors.Is(err, council.ErrStale) {
		t.Fatal("stale proposal counted", err)
	}
	var commits, votes, audits int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM mailbox.council_commits WHERE decision_id=$1`, id).Scan(&commits); err != nil || commits != 1 {
		t.Fatal("duplicate acceptance", err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM mailbox.council_votes WHERE decision_id=$1`, id).Scan(&votes); err != nil || votes != 3 {
		t.Fatal("duplicate vote evidence", err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM mailbox.audit_events WHERE action='council.accepted' AND target_id=$1`, id).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("duplicate majority audit", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM mailbox.council_votes WHERE decision_id=$1`, id); err == nil {
		t.Fatal("vote evidence deletion allowed")
	}
	if _, err = queue.Control(ctx, human, project, root.ID, "pause", 1); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Vote(ctx, tokens[0], id, 1, hash, council.Approve); !errors.Is(err, councilstore.ErrStale) {
		t.Fatal("paused root accepted action", err)
	}
	// A second root exercises coordinator races and all three durable rounds.
	exhaustedRoot, err := queue.Create(ctx, human, backlog.Input{ProjectID: project, Key: "exhausted", Title: "Tied plan", Description: "No human tie-break"})
	if err != nil {
		t.Fatal(err)
	}
	content.RootID = exhaustedRoot.ID
	nextContract, err := contractStore.Publish(ctx, tokens[0], content)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Actions[0].Target = nextContract.ID
	if _, err = pool.Exec(ctx, `INSERT INTO mailbox.contract_validations(project_id,contract_id,contract_hash,validator_runner_id) VALUES($1,$2,$3,$4)`, project, nextContract.ID, nextContract.Hash, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.project_runners SET credential_expires_at=clock_timestamp()+interval '15 minutes' WHERE id=$1`, ids[3]); err != nil {
		t.Fatal(err)
	}
	tied, err := store.OpenPlan(ctx, tokens[0], nextContract.ID, proposal)
	if err != nil {
		t.Fatal(err)
	}
	type leaseResult struct {
		lease councilstore.Lease
		token string
		err   error
	}
	leases := make(chan leaseResult, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			l, e := store.Coordinate(ctx, tokens[i], tied.Snapshot.ID)
			leases <- leaseResult{l, tokens[i], e}
		}(i)
	}
	winner, loser := <-leases, <-leases
	if winner.err != nil {
		winner, loser = loser, winner
	}
	if winner.err != nil || !errors.Is(loser.err, councilstore.ErrStale) {
		t.Fatal("coordinator ownership race", winner.err, loser.err)
	}
	coordinatorToken, epoch := winner.token, winner.lease.Epoch
	for round := 1; round <= council.MaxRounds; round++ {
		proposalHash := tied.Snapshot.Rounds[round-1].Hash
		for i := 0; i < 4; i++ {
			choice := council.Object
			if i < 2 {
				choice = council.Approve
			}
			tied, err = store.Vote(ctx, tokens[i], tied.Snapshot.ID, round, proposalHash, choice)
			if err != nil {
				t.Fatal(err)
			}
		}
		if round < council.MaxRounds {
			if tied.Snapshot.Status != council.NeedsRevision {
				t.Fatal("tie accepted")
			}
			if round == 1 {
				if _, err = pool.Exec(ctx, `UPDATE mailbox.council_coordinators SET expires_at=clock_timestamp()-interval '1 second' WHERE decision_id=$1`, tied.Snapshot.ID); err != nil {
					t.Fatal(err)
				}
				lease, e := store.Coordinate(ctx, tokens[2], tied.Snapshot.ID)
				if e != nil || lease.Epoch <= epoch {
					t.Fatal("lease takeover missing fence", e)
				}
				if _, err = store.Revise(ctx, coordinatorToken, tied.Snapshot.ID, epoch, round, proposalHash, proposal); !errors.Is(err, councilstore.ErrStale) {
					t.Fatal("expired coordinator revised proposal", err)
				}
				coordinatorToken, epoch = tokens[2], lease.Epoch
			}
			revised := proposal
			revised.Rationale = "Continuing discussion"
			tied, err = store.Revise(ctx, coordinatorToken, tied.Snapshot.ID, epoch, round, proposalHash, revised)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if tied.Snapshot.Status != council.Blocked {
		t.Fatal("third tie did not block")
	}
	again, err := restarted.OpenPlan(ctx, tokens[0], nextContract.ID, proposal)
	if err != nil || again.Snapshot.ID != tied.Snapshot.ID || len(again.Snapshot.Rounds) != 3 {
		t.Fatal("restart reset exhausted rounds", err)
	}
	cosmetic := proposal
	cosmetic.Rationale = "A fresh label"
	if _, err = restarted.ReconsiderPlan(ctx, tokens[0], nextContract.ID, cosmetic); !errors.Is(err, council.ErrUnchanged) {
		t.Fatal("cosmetic restart evaded round limit", err)
	}
	material := proposal
	material.Actions = append([]council.Action(nil), proposal.Actions...)
	material.Actions[0].Value = "Implement verified scope with a revised technical approach"
	reconsidered, err := restarted.ReconsiderPlan(ctx, tokens[0], nextContract.ID, material)
	if err != nil || reconsidered.Snapshot.PreviousID != tied.Snapshot.ID || reconsidered.Snapshot.Status != council.Discussing {
		t.Fatal("material reconsideration lost provenance", err)
	}
	previousID := reconsidered.Snapshot.ID
	if _, err = queue.Revise(ctx, human, exhaustedRoot.ID, 1, backlog.Input{ProjectID: project, Key: "scope-revision", Title: "Revised tied plan", Description: "New authoritative scope"}); err != nil {
		t.Fatal(err)
	}
	content.RootRevision = 2
	revisedContract, err := contractStore.Publish(ctx, tokens[0], content)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO mailbox.contract_validations(project_id,contract_id,contract_hash,validator_runner_id) VALUES($1,$2,$3,$4)`, project, revisedContract.ID, revisedContract.Hash, ids[0]); err != nil {
		t.Fatal(err)
	}
	scopeProposal := material
	scopeProposal.Actions = append([]council.Action(nil), material.Actions...)
	scopeProposal.Actions[0].Target = revisedContract.ID
	reconsidered, err = restarted.ReconsiderPlan(ctx, tokens[0], revisedContract.ID, scopeProposal)
	if err != nil || reconsidered.Snapshot.PreviousID != previousID {
		t.Fatal("new authoritative scope could not replace in-flight plan", err)
	}
	if _, err = restarted.Vote(ctx, tokens[0], previousID, 1, again.Snapshot.Rounds[0].Hash, council.Approve); !errors.Is(err, councilstore.ErrStale) {
		t.Fatal("superseded input counted a vote", err)
	}
	authority.allowed[101] = false
	if _, err = restarted.Vote(ctx, tokens[0], reconsidered.Snapshot.ID, 1, reconsidered.Snapshot.Rounds[0].Hash, council.Approve); !errors.Is(err, councilstore.ErrForbidden) {
		t.Fatal("removed collaborator voted", err)
	}
	authority.allowed[101] = true
	var raw []byte
	if err = pool.QueryRow(ctx, `SELECT snapshot FROM mailbox.council_decisions WHERE id=$1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var stored council.Snapshot
	if json.Unmarshal(raw, &stored) != nil || stored.Status != council.Accepted {
		t.Fatal("restart lost accepted history")
	}
	history, err := restarted.Get(ctx, tokens[0], id)
	if err != nil || history.Snapshot.Status != council.Accepted {
		t.Fatal("fenced historical decision unavailable", err)
	}
	forged, err := council.Restore(reconsidered.Snapshot, reconsidered.Material)
	if err != nil {
		t.Fatal(err)
	}
	forgedHash := forged.Snapshot().Rounds[0].Hash
	for _, member := range forged.Snapshot().Members[:3] {
		if err = forged.Cast(member, 1, forgedHash, council.Approve); err != nil {
			t.Fatal(err)
		}
	}
	forgedJSON, _ := json.Marshal(forged.Snapshot())
	if _, err = pool.Exec(ctx, `UPDATE mailbox.council_decisions SET snapshot=$2 WHERE id=$1`, reconsidered.Snapshot.ID, forgedJSON); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Get(ctx, tokens[0], reconsidered.Snapshot.ID); !errors.Is(err, councilstore.ErrUnavailable) {
		t.Fatal("snapshot-only forged majority accepted", err)
	}
}
