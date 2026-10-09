package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/contracts"
	"github.com/petarnenov/bot-space/internal/controlevents"
	"github.com/petarnenov/bot-space/internal/council"
	"github.com/petarnenov/bot-space/internal/councilstore"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/workallocation"
)

type councilVerifyHook func(context.Context, repositoryaccess.Repository, int64, bool) error

func (f councilVerifyHook) Verify(ctx context.Context, repo repositoryaccess.Repository, actor int64, force bool) error {
	return f(ctx, repo, actor, force)
}

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
	content := contracts.Content{ProjectID: project, RootID: root.ID, RootRevision: 1, RepositoryID: 42, BaseCommit: strings.Repeat("a", 40), Change: "feature", Tasks: []string{"1.1", "1.2"}, Scenarios: []string{"work::Output::Success"}, Artifacts: map[string]string{}}
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
	architectEvents, err := (&controlevents.Store{Pool: pool, Identities: identities}).Page(ctx, tokens[0])
	if err != nil || len(architectEvents) == 0 || architectEvents[0].Frame.GetCouncil() == nil {
		t.Fatal("council changes were not delivered", err)
	}
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
	identities.Authority = councilVerifyHook(func(ctx context.Context, repo repositoryaccess.Repository, actor int64, force bool) error {
		if force {
			_, e := pool.Exec(ctx, `UPDATE mailbox.orchestration_projects SET repository_name='moved' WHERE id=$1`, project)
			return e
		}
		return nil
	})
	if _, err = restarted.Get(ctx, tokens[0], id); !errors.Is(err, councilstore.ErrForbidden) {
		t.Fatal("repository mapping changed after verification but transaction retained authority", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.orchestration_projects SET repository_name='project' WHERE id=$1`, project); err != nil {
		t.Fatal(err)
	}
	identities.Authority = authority
	// A new verified commit with the same human revision replaces the plan
	// without making the old contract itself stale. Current-head fencing must
	// independently reject votes and coordination for that superseded plan.
	oldPlanID, oldPlanHash := reconsidered.Snapshot.ID, reconsidered.Snapshot.Rounds[0].Hash
	content.BaseCommit = strings.Repeat("d", 40)
	newPlanContract, err := contractStore.Publish(ctx, tokens[0], content)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO mailbox.contract_validations(project_id,contract_id,contract_hash,validator_runner_id) VALUES($1,$2,$3,$4)`, project, newPlanContract.ID, newPlanContract.Hash, ids[0]); err != nil {
		t.Fatal(err)
	}
	nextPlan := scopeProposal
	nextPlan.Actions = append([]council.Action(nil), scopeProposal.Actions...)
	nextPlan.Actions[0].Target = newPlanContract.ID
	reconsidered, err = restarted.ReconsiderPlan(ctx, tokens[0], newPlanContract.ID, nextPlan)
	if err != nil || reconsidered.Snapshot.PreviousID != oldPlanID {
		t.Fatal("new commit lost replacement history", err)
	}
	if _, err = restarted.Vote(ctx, tokens[0], oldPlanID, 1, oldPlanHash, council.Approve); !errors.Is(err, councilstore.ErrStale) {
		t.Fatal("superseded plan gained votes", err)
	}
	if _, err = restarted.Coordinate(ctx, tokens[0], oldPlanID); !errors.Is(err, councilstore.ErrStale) {
		t.Fatal("superseded plan acquired a coordinator", err)
	}
	if _, err = restarted.Get(ctx, tokens[0], oldPlanID); err != nil {
		t.Fatal("superseded history disappeared", err)
	}
	revisedContract = newPlanContract
	var executor string
	if err = pool.QueryRow(ctx, `INSERT INTO mailbox.project_runners(project_id,owner_github_id,public_key,role) VALUES($1,101,decode($2,'hex'),'executor') RETURNING id::text`, project, security.Hash("executor-machine")).Scan(&executor); err != nil {
		t.Fatal(err)
	}
	allocationProposal := council.Proposal{Actions: []council.Action{{Kind: "assign", Target: executor, Value: "1.1"}}, Rationale: "Assign task 1.1"}
	allocation, err := restarted.OpenAllocation(ctx, tokens[0], revisedContract.ID, "1.1", allocationProposal)
	if err != nil {
		t.Fatal(err)
	}
	otherProposal := council.Proposal{Actions: []council.Action{{Kind: "assign", Target: executor, Value: "1.2"}}, Rationale: "Assign task 1.2"}
	otherAllocation, err := restarted.OpenAllocation(ctx, tokens[0], revisedContract.ID, "1.2", otherProposal)
	if err != nil || otherAllocation.Snapshot.ID == allocation.Snapshot.ID || otherAllocation.Subject != "task:1.2" {
		t.Fatal("independent tasks shared one decision", err)
	}
	if _, err = restarted.OpenAllocation(ctx, tokens[0], revisedContract.ID, "1.2", allocationProposal); !errors.Is(err, council.ErrInvalid) {
		t.Fatal("proposal changed its bound task", err)
	}
	missingTask := council.Proposal{Actions: []council.Action{{Kind: "assign", Target: executor, Value: "99.1"}}}
	if _, err = restarted.OpenAllocation(ctx, tokens[0], revisedContract.ID, "99.1", missingTask); !errors.Is(err, council.ErrInvalid) {
		t.Fatal("missing OpenSpec task deliberated", err)
	}
	var foreignProject, foreignExecutor string
	if err = pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,43,101,'owner','other') RETURNING id::text`, workspace.ID).Scan(&foreignProject); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO mailbox.project_runners(project_id,owner_github_id,public_key,role) VALUES($1,101,decode($2,'hex'),'executor') RETURNING id::text`, foreignProject, security.Hash("executor-machine")).Scan(&foreignExecutor); err != nil {
		t.Fatal(err)
	}
	foreignProposal := council.Proposal{Actions: []council.Action{{Kind: "assign", Target: foreignExecutor, Value: "1.1"}}}
	if _, err = restarted.OpenAllocation(ctx, tokens[0], revisedContract.ID, "1.1", foreignProposal); !errors.Is(err, councilstore.ErrForbidden) {
		t.Fatal("foreign-project executor accepted", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.project_runners SET active=false WHERE id=$1`, executor); err != nil {
		t.Fatal(err)
	}
	otherHash := otherAllocation.Snapshot.Rounds[0].Hash
	if _, err = restarted.Vote(ctx, tokens[0], otherAllocation.Snapshot.ID, 1, otherHash, council.Object); err != nil {
		t.Fatal("withdrawn executor prevented an objection", err)
	}
	if _, err = restarted.Vote(ctx, tokens[1], otherAllocation.Snapshot.ID, 1, otherHash, council.Approve); !errors.Is(err, councilstore.ErrForbidden) {
		t.Fatal("withdrawn executor gained approval", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.project_runners SET active=true WHERE id=$1`, executor); err != nil {
		t.Fatal(err)
	}
	allocationHash := allocation.Snapshot.Rounds[0].Hash
	for i := 0; i < 3; i++ {
		allocation, err = restarted.Vote(ctx, tokens[i], allocation.Snapshot.ID, 1, allocationHash, council.Approve)
		if err != nil {
			t.Fatal(err)
		}
	}
	if allocation.Snapshot.Status != council.Accepted {
		t.Fatal("allocation majority not persisted")
	}
	gate := func(id, kind, subject, digest string, want bool) {
		t.Helper()
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, e = councilstore.LockAccepted(ctx, tx, project, id, kind, subject, digest)
		if (e == nil) != want {
			t.Fatal("incorrect exact-subject acceptance gate", kind, subject, e)
		}
	}
	gate(allocation.Snapshot.ID, "allocation", "task:1.1", revisedContract.Hash, false)
	planHash := reconsidered.Snapshot.Rounds[0].Hash
	for i := 0; i < 3; i++ {
		reconsidered, err = restarted.Vote(ctx, tokens[i], reconsidered.Snapshot.ID, 1, planHash, council.Approve)
		if err != nil {
			t.Fatal(err)
		}
	}
	gate(allocation.Snapshot.ID, "allocation", "task:1.1", revisedContract.Hash, true)
	gate(allocation.Snapshot.ID, "allocation", "task:1.2", revisedContract.Hash, false)
	gate(allocation.Snapshot.ID, "review", "task:1.1", revisedContract.Hash, false)
	gate(allocation.Snapshot.ID, "allocation", "task:1.1", strings.Repeat("f", 64), false)
	gate(otherAllocation.Snapshot.ID, "allocation", "task:1.2", revisedContract.Hash, false)
	// Two project registrations for one key share exactly one executor slot.
	foreignRoot, err := queue.Create(ctx, human, backlog.Input{ProjectID: foreignProject, Key: "foreign-slot", Title: "Other project work", Description: "One shared executor"})
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := security.Secret()
	foreignArchToken := runneridentity.TokenPrefix + secret
	var foreignArch string
	if err = pool.QueryRow(ctx, `INSERT INTO mailbox.project_runners(project_id,owner_github_id,public_key,role,credential_hash,credential_expires_at) SELECT $1,101,public_key,'architect',$2,clock_timestamp()+interval '15 minutes' FROM mailbox.project_runners WHERE id=$3 RETURNING id::text`, foreignProject, security.Hash(foreignArchToken), ids[0]).Scan(&foreignArch); err != nil {
		t.Fatal(err)
	}
	foreignContent := content
	foreignContent.ProjectID = foreignProject
	foreignContent.RootID = foreignRoot.ID
	foreignContent.RootRevision = 1
	foreignContent.RepositoryID = 43
	foreignContract, err := contractStore.Publish(ctx, foreignArchToken, foreignContent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO mailbox.contract_validations(project_id,contract_id,contract_hash,validator_runner_id) VALUES($1,$2,$3,$4)`, foreignProject, foreignContract.ID, foreignContract.Hash, foreignArch); err != nil {
		t.Fatal(err)
	}
	foreignPlan, err := restarted.OpenPlan(ctx, foreignArchToken, foreignContract.ID, council.Proposal{Actions: []council.Action{{Kind: "plan", Target: foreignContract.ID, Value: "Implement other project"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Vote(ctx, foreignArchToken, foreignPlan.Snapshot.ID, 1, foreignPlan.Snapshot.Rounds[0].Hash, council.Approve); err != nil {
		t.Fatal(err)
	}
	foreignDecision, err := restarted.OpenAllocation(ctx, foreignArchToken, foreignContract.ID, "1.1", council.Proposal{Actions: []council.Action{{Kind: "assign", Target: foreignExecutor, Value: "1.1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Vote(ctx, foreignArchToken, foreignDecision.Snapshot.ID, 1, foreignDecision.Snapshot.Rounds[0].Hash, council.Approve); err != nil {
		t.Fatal(err)
	}
	executorTokens := []string{}
	for _, runner := range []string{executor, foreignExecutor} {
		s, _ := security.Secret()
		token := runneridentity.TokenPrefix + s
		executorTokens = append(executorTokens, token)
		if _, err = pool.Exec(ctx, `UPDATE mailbox.project_runners SET credential_hash=$2,credential_expires_at=clock_timestamp()+interval '15 minutes' WHERE id=$1`, runner, security.Hash(token)); err != nil {
			t.Fatal(err)
		}
	}
	allocator := &workallocation.Store{Pool: pool, Identities: identities}
	client := workallocation.Client{Agent: "codex", Version: "test-fixture", Tools: []string{"git", "openspec"}}
	for _, token := range executorTokens {
		if err = allocator.Presence(ctx, token, true, client); err != nil {
			t.Fatal(err)
		}
		if busy, e := allocator.Busy(ctx, token); e != nil || busy {
			t.Fatal("new executor advertised busy", e)
		}
	}
	type assignmentResult struct {
		assignment      workallocation.Assignment
		token, decision string
		err             error
	}
	assigned := make(chan assignmentResult, 2)
	for _, request := range []struct{ token, decision string }{{tokens[0], allocation.Snapshot.ID}, {foreignArchToken, foreignDecision.Snapshot.ID}} {
		go func(token, decision string) {
			a, e := allocator.Assign(ctx, token, decision)
			assigned <- assignmentResult{a, token, decision, e}
		}(request.token, request.decision)
	}
	accepted, rejected := <-assigned, <-assigned
	if accepted.err != nil {
		accepted, rejected = rejected, accepted
	}
	if accepted.err != nil || !errors.Is(rejected.err, workallocation.ErrOccupied) {
		t.Fatal("cross-project assignments both reserved one machine", accepted.err, rejected.err)
	}
	retried, err := allocator.Assign(ctx, accepted.token, accepted.decision)
	if err != nil || retried.ID != accepted.assignment.ID || retried.Generation != 1 {
		t.Fatal("assignment retry created new work", err)
	}
	for _, token := range executorTokens {
		if busy, e := allocator.Busy(ctx, token); e != nil || !busy {
			t.Fatal("occupied slot not shared across scopes", e)
		}
		if err = allocator.Presence(ctx, token, true, client); err != nil {
			t.Fatal("same-client busy heartbeat failed", err)
		}
	}
	changedClient := client
	changedClient.Agent = "copilot"
	if err = allocator.Presence(ctx, executorTokens[0], true, changedClient); !errors.Is(err, workallocation.ErrOccupied) {
		t.Fatal("occupied machine switched providers", err)
	}
	executorToken := executorTokens[0]
	wrongScopeToken := executorTokens[1]
	if accepted.assignment.Project == foreignProject {
		executorToken, wrongScopeToken = wrongScopeToken, executorToken
	}
	workAttempt, err := allocator.Begin(ctx, executorToken, accepted.assignment.ID, client)
	if err != nil || workAttempt.Number != 1 || workAttempt.Session != "" || workAttempt.State != "starting" {
		t.Fatal("initial attempt failed", err)
	}
	retryAttempt, err := allocator.Begin(ctx, executorToken, accepted.assignment.ID, client)
	if err != nil || retryAttempt.ID != workAttempt.ID {
		t.Fatal("begin retry reset context", err)
	}
	if _, err = allocator.Begin(ctx, wrongScopeToken, accepted.assignment.ID, client); !errors.Is(err, workallocation.ErrForbidden) {
		t.Fatal("another project started the assignment", err)
	}
	if _, err = allocator.Begin(ctx, executorToken, accepted.assignment.ID, changedClient); !errors.Is(err, workallocation.ErrInvalid) {
		t.Fatal("attempt switched provider settings", err)
	}
	if _, err = allocator.Renew(ctx, executorToken, accepted.assignment.ID, "", workAttempt.AuthorityEpoch); err != nil {
		t.Fatal("native initialization could not keep authority alive", err)
	}
	bound, err := allocator.BindSession(ctx, executorToken, accepted.assignment.ID, "native-fixture-session", workAttempt.AuthorityEpoch)
	if err != nil || bound.State != "running" || bound.Session != "native-fixture-session" {
		t.Fatal("session binding failed", err)
	}
	if _, err = allocator.BindSession(ctx, executorToken, accepted.assignment.ID, "another-session", workAttempt.AuthorityEpoch); !errors.Is(err, workallocation.ErrInvalid) {
		t.Fatal("session replacement allowed", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.work_attempts SET session_id='rewritten' WHERE id=$1`, workAttempt.ID); err == nil {
		t.Fatal("database session replacement allowed")
	}
	questionInput := workallocation.QuestionContext{Question: "Which locking strategy should I use?", Branch: "openspec/feature", Commit: strings.Repeat("a", 40), Tried: []string{"Inspected allocation invariants"}, Checks: []string{"Current tests pass"}, Diff: "No changes yet"}
	question, err := allocator.Ask(ctx, executorToken, accepted.assignment.ID, bound.Session, "question-1", workAttempt.AuthorityEpoch, questionInput)
	if err != nil || question.Attempt != workAttempt.ID || question.Session != bound.Session {
		t.Fatal("exact-session question failed", err)
	}
	duplicate, err := allocator.Ask(ctx, executorToken, accepted.assignment.ID, bound.Session, "another-label", workAttempt.AuthorityEpoch, questionInput)
	if err != nil || duplicate.ID != question.ID {
		t.Fatal("request label reset question identity", err)
	}
	changedQuestion := questionInput
	changedQuestion.Question = "A different question"
	if _, err = allocator.Ask(ctx, executorToken, accepted.assignment.ID, bound.Session, "another-label", workAttempt.AuthorityEpoch, changedQuestion); !errors.Is(err, workallocation.ErrQuestionConflict) {
		t.Fatal("question alias payload changed", err)
	}
	if _, err = allocator.Ask(ctx, executorToken, accepted.assignment.ID, "wrong-session", "wrong", workAttempt.AuthorityEpoch, questionInput); !errors.Is(err, workallocation.ErrInvalid) {
		t.Fatal("wrong session asked a question", err)
	}
	answerProposal := council.Proposal{Actions: []council.Action{{Kind: "answer", Target: question.ID, Value: "Keep the exact task session and serialize the machine slot."}}}
	answerDecision, err := restarted.OpenAnswer(ctx, accepted.token, question.ID, answerProposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = allocator.AcceptAnswer(ctx, executorToken, accepted.assignment.ID, question.ID, answerDecision.Snapshot.ID, bound.Session, workAttempt.AuthorityEpoch); !errors.Is(err, councilstore.ErrStale) {
		t.Fatal("answer resumed without majority", err)
	}
	answerTokens := tokens[:3]
	if accepted.assignment.Project == foreignProject {
		answerTokens = []string{foreignArchToken}
	}
	for _, token := range answerTokens {
		answerDecision, err = restarted.Vote(ctx, token, answerDecision.Snapshot.ID, 1, answerDecision.Snapshot.Rounds[0].Hash, council.Approve)
		if err != nil {
			t.Fatal(err)
		}
	}
	response, err := allocator.AcceptAnswer(ctx, executorToken, accepted.assignment.ID, question.ID, answerDecision.Snapshot.ID, bound.Session, workAttempt.AuthorityEpoch)
	if err != nil || response.Session != bound.Session || response.Text != answerProposal.Actions[0].Value {
		t.Fatal("majority answer lost exact continuation", err)
	}
	repeatedResponse, err := allocator.AcceptAnswer(ctx, executorToken, accepted.assignment.ID, question.ID, answerDecision.Snapshot.ID, bound.Session, workAttempt.AuthorityEpoch)
	if err != nil || repeatedResponse.Text != response.Text {
		t.Fatal("answer retry changed history", err)
	}
	delivered, err := (&controlevents.Store{Pool: pool, Identities: identities}).Page(ctx, executorToken)
	if err != nil || len(delivered) != 2 || delivered[0].Frame.GetAssignment().GetAssignmentId() != accepted.assignment.ID || delivered[1].Frame.GetAnswer().GetSessionId() != bound.Session {
		t.Fatal("atomic assignment/answer events missing", err)
	}
	delayedQuestion, err := allocator.Ask(ctx, executorToken, accepted.assignment.ID, bound.Session, "question-1", workAttempt.AuthorityEpoch, questionInput)
	if err != nil || delayedQuestion.ID != question.ID || !delayedQuestion.Answered {
		t.Fatal("delayed question retry re-entered wait", err)
	}
	currentAttempt, err := allocator.Begin(ctx, executorToken, accepted.assignment.ID, client)
	if err != nil || currentAttempt.State != "running" || currentAttempt.Session != bound.Session {
		t.Fatal("answered question lost continuation state", err)
	}
	if _, err = allocator.AcceptAnswer(ctx, executorToken, accepted.assignment.ID, question.ID, answerDecision.Snapshot.ID, "wrong-session", workAttempt.AuthorityEpoch); !errors.Is(err, workallocation.ErrInvalid) {
		t.Fatal("old answer changed native session", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM mailbox.executor_questions WHERE id=$1`, question.ID); err == nil {
		t.Fatal("question context deleted")
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.executor_question_answers SET answer='rewritten' WHERE question_id=$1`, question.ID); err == nil {
		t.Fatal("answer history rewritten")
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.work_assignments SET created_at=clock_timestamp()-interval '2 days' WHERE id=$1`, accepted.assignment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.work_attempts SET created_at=clock_timestamp()-interval '2 days' WHERE id=$1`, workAttempt.ID); err != nil {
		t.Fatal(err)
	}
	renewed, err := allocator.Renew(ctx, executorToken, accepted.assignment.ID, bound.Session, workAttempt.AuthorityEpoch)
	if err != nil || renewed.ID != workAttempt.ID || renewed.Session != bound.Session {
		t.Fatal("old task age imposed execution deadline", err)
	}
	if _, err = allocator.Renew(ctx, executorToken, accepted.assignment.ID, "wrong-session", workAttempt.AuthorityEpoch); !errors.Is(err, workallocation.ErrInvalid) {
		t.Fatal("wrong session renewed authority", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.work_assignments SET state='question_wait' WHERE id=$1`, accepted.assignment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.work_attempts SET state='question_wait' WHERE id=$1`, workAttempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = allocator.Renew(ctx, executorToken, accepted.assignment.ID, bound.Session, workAttempt.AuthorityEpoch); err != nil {
		t.Fatal("question wait lost same-session authority", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.work_attempts SET authority_until=clock_timestamp()-interval '1 second' WHERE id=$1`, workAttempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = allocator.Renew(ctx, executorToken, accepted.assignment.ID, bound.Session, workAttempt.AuthorityEpoch); !errors.Is(err, workallocation.ErrAuthorityLost) {
		t.Fatal("expired authority was resurrected", err)
	}
	if _, err = allocator.Begin(ctx, executorToken, accepted.assignment.ID, client); !errors.Is(err, workallocation.ErrAuthorityLost) {
		t.Fatal("interruption created a fresh task context", err)
	}
	for _, state := range []string{"question_wait", "interrupted"} {
		if _, err = pool.Exec(ctx, `UPDATE mailbox.work_assignments SET state=$2 WHERE id=$1`, accepted.assignment.ID, state); err != nil {
			t.Fatal(err)
		}
		if _, err = allocator.Assign(ctx, rejected.token, rejected.decision); !errors.Is(err, workallocation.ErrOccupied) {
			t.Fatal("waiting or uncertain work freed its slot", state, err)
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE mailbox.executor_presence SET lease_until=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if _, err = allocator.Assign(ctx, rejected.token, rejected.decision); !errors.Is(err, workallocation.ErrOccupied) {
		t.Fatal("presence expiration freed uncertain work", err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM mailbox.work_assignments`).Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent reservation created extra assignments", err)
	}
	if _, err = queue.Control(ctx, human, project, exhaustedRoot.ID, "pause", 1); err != nil {
		t.Fatal(err)
	}
	var stops int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM mailbox.control_outbox WHERE project_id=$1 AND event_key=$2`, project, "root:"+exhaustedRoot.ID+":2").Scan(&stops); err != nil || stops < 1 {
		t.Fatal("root pause did not publish stop events", err)
	}
	gate(allocation.Snapshot.ID, "allocation", "task:1.1", revisedContract.Hash, false)
	forged, err := council.Restore(otherAllocation.Snapshot, otherAllocation.Material)
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
	if _, err = pool.Exec(ctx, `UPDATE mailbox.council_decisions SET snapshot=$2 WHERE id=$1`, otherAllocation.Snapshot.ID, forgedJSON); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Get(ctx, tokens[0], otherAllocation.Snapshot.ID); !errors.Is(err, councilstore.ErrUnavailable) {
		t.Fatal("snapshot-only forged majority accepted", err)
	}
}
