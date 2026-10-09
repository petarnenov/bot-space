package integration

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/tasks"
)

const runnerOne = "11111111-1111-4111-8111-111111111111"
const runnerTwo = "22222222-2222-4222-8222-222222222222"

func TestTaskSchemaRejectsCrossWorkspaceReferences(t *testing.T) {
	f := mailSetup(t)
	other, err := f.teams.Bootstrap(f.ctx, 201, "foreign-task-team")
	if err != nil {
		t.Fatal(err)
	}
	owner := user(t, f.ctx, f.pool, 201)
	foreign, _, _ := agent(t, f.ctx, f.agents, other, owner, "foreign")
	message, err := f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "schema-request", Text: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	sql := "WITH generated AS (SELECT gen_random_uuid() AS id) INSERT INTO mailbox.tasks (id,workspace_id,from_agent_id,to_agent_id,root_task_id,ancestor_agents,idempotency_key,fingerprint,instruction,deadline,request_message_id) SELECT id,$1,$2,$3,id,ARRAY[$2::uuid],'foreign',repeat('a',64),'synthetic',now()+interval '1 hour',$4 FROM generated"
	_, err = f.pool.Exec(f.ctx, sql, f.workspace.ID, f.a.ID, foreign.ID, message.ID)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23503" {
		t.Fatal("tenant composite foreign key did not reject foreign agent")
	}
	var count int
	if f.pool.QueryRow(f.ctx, "SELECT count(*) FROM mailbox.tasks").Scan(&count) != nil || count != 0 {
		t.Fatal("foreign task persisted")
	}
}

func TestTasksSubmitClaimCompleteAtomicMailbox(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	input := tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "delegation", Instruction: "Compute a synthetic result"}
	submitted, err := store.Submit(f.ctx, f.ta, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := store.Submit(f.ctx, f.ta, input)
	if err != nil || retry.ID != submitted.ID {
		t.Fatal("submission retry duplicated work")
	}
	changed := input
	changed.Instruction = "different"
	if _, err := store.Submit(f.ctx, f.ta, changed); !errors.Is(err, tasks.ErrConflict) {
		t.Fatal("changed payload accepted")
	}
	page, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != submitted.RequestMessageID || page.Messages[0].AcknowledgedAt != nil {
		t.Fatal("task request not atomically delivered")
	}
	owner, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(f.ctx, f.tb, runnerOne, owner.Generation)
	if err != nil || claimed == nil || claimed.ID != submitted.ID || claimed.Status != "running" {
		t.Fatalf("claim failed: %v", err)
	}
	if _, err := store.Heartbeat(f.ctx, f.tb, runnerTwo); !errors.Is(err, tasks.ErrLease) {
		t.Fatal("second machine acquired active agent")
	}
	if next, err := store.Claim(f.ctx, f.tb, runnerOne, owner.Generation); err != nil || next != nil {
		t.Fatal("one agent claimed concurrent work")
	}
	if _, err := store.Renew(f.ctx, f.tb, claimed.ID, runnerOne, claimed.Generation, owner.Generation); err != nil {
		t.Fatal(err)
	}
	complete := tasks.CompleteInput{TaskID: claimed.ID, RunnerID: runnerOne, Generation: claimed.Generation, RunnerGeneration: owner.Generation, Result: strings.Repeat("R", tasks.MaxResultBytes)}
	result, err := store.Complete(f.ctx, f.tb, complete)
	if err != nil || result.Status != "completed" || result.Result == nil || len(*result.Result) != tasks.MaxResultBytes || result.ReplyMessageID == nil {
		t.Fatalf("complete failed: %v", err)
	}
	again, err := store.Complete(f.ctx, f.tb, complete)
	if err != nil || again.ReplyMessageID == nil || *again.ReplyMessageID != *result.ReplyMessageID {
		t.Fatal("delivery retry changed result")
	}
	read, err := store.Get(f.ctx, f.ta, result.ID)
	if err != nil || read.Result == nil || *read.Result != complete.Result {
		t.Fatal("sender did not get correlated actual output")
	}
	inbox, err := f.store.Read(f.ctx, f.ta, mailbox.ReadInput{})
	if err != nil || len(inbox.Messages) != 1 || inbox.Messages[0].InReplyTo == nil || *inbox.Messages[0].InReplyTo != submitted.RequestMessageID {
		t.Fatal("completion reply missing")
	}
	page, err = f.store.Read(f.ctx, f.tb, mailbox.ReadInput{})
	if err != nil || len(page.Messages) != 0 {
		t.Fatal("completed request not acknowledged")
	}
}

func TestTasksConcurrentKeysClaimsAndRevocation(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	var wg sync.WaitGroup
	ids := make(chan string, 6)
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "parallel", Instruction: "synthetic"})
			ids <- task.ID
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for x := range ids {
		if id == "" {
			id = x
		}
		if id != x {
			t.Fatal("duplicate task")
		}
	}
	own, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(f.ctx, f.tb, runnerOne, own.Generation)
	if err != nil || claimed == nil {
		t.Fatal("claim failed")
	}
	if err = f.agents.Revoke(f.ctx, f.workspace.ID, f.member.ID, f.b.ID, f.cb.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Renew(f.ctx, f.tb, id, runnerOne, claimed.Generation, own.Generation); !errors.Is(err, tasks.ErrForbidden) {
		t.Fatal("revoked worker renewed")
	}
	if _, err = store.Get(f.ctx, f.tb, id); !errors.Is(err, tasks.ErrForbidden) {
		t.Fatal("revoked agent read task")
	}
}

func TestTasksLeaseLossAndExplicitRetrySafety(t *testing.T) {
	for _, safe := range []bool{false, true} {
		f := mailSetup(t)
		store := tasks.New(f.store)
		task, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "lease", Instruction: "synthetic", RetrySafe: safe})
		if err != nil {
			t.Fatal(err)
		}
		owner, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
		if err != nil {
			t.Fatal(err)
		}
		attempt, err := store.Claim(f.ctx, f.tb, runnerOne, owner.Generation)
		if err != nil || attempt == nil {
			t.Fatal(err)
		}
		if _, err = f.pool.Exec(f.ctx, "UPDATE mailbox.tasks SET lease_until=now()-interval '1 second' WHERE id=$1", task.ID); err != nil {
			t.Fatal(err)
		}
		view, err := store.Get(f.ctx, f.ta, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		expected := "interrupted"
		if safe {
			expected = "queued"
		}
		if view.Status != expected {
			t.Fatal("unsafe work requeued or safe retry lost")
		}
		late := tasks.CompleteInput{TaskID: task.ID, RunnerID: runnerOne, Generation: attempt.Generation, RunnerGeneration: owner.Generation, Result: "late"}
		if _, err := store.Complete(f.ctx, f.tb, late); !errors.Is(err, tasks.ErrLease) {
			t.Fatal("stale attempt completed")
		}
		if safe {
			next, err := store.Claim(f.ctx, f.tb, runnerOne, owner.Generation)
			if err != nil || next == nil || next.Generation <= attempt.Generation {
				t.Fatal("retry not fenced")
			}
		}
	}
}

func TestTasksCancellationDependenciesAndIsolation(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	third := join(t, f.ctx, f.pool, f.teams, f.workspace, f.owner, 103, "member")
	c, _, tc := agent(t, f.ctx, f.agents, f.workspace, third, "third")
	root, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "root", Instruction: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	own, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.Claim(f.ctx, f.tb, runnerOne, own.Generation)
	if err != nil || parent == nil {
		t.Fatal(err)
	}
	if _, err := store.Get(f.ctx, tc, root.ID); !errors.Is(err, tasks.ErrNotFound) {
		t.Fatal("third party read task")
	}
	if _, err := store.Submit(f.ctx, f.tb, tasks.SubmitInput{ToAgentID: f.a.ID, IdempotencyKey: "cycle", Instruction: "synthetic", ParentTaskID: &root.ID, ParentGeneration: parent.Generation}); !errors.Is(err, tasks.ErrDependency) {
		t.Fatal("ancestor cycle accepted")
	}
	child, err := store.Submit(f.ctx, f.tb, tasks.SubmitInput{ToAgentID: c.ID, IdempotencyKey: "child", Instruction: "synthetic", ParentTaskID: &root.ID, ParentGeneration: parent.Generation})
	if err != nil || child.RootTaskID != root.ID || child.Depth != 2 || child.Deadline.After(root.Deadline) {
		t.Fatalf("child correlation failed: %v", err)
	}
	if _, err := store.Cancel(f.ctx, f.tb, root.ID); !errors.Is(err, tasks.ErrNotFound) {
		t.Fatal("recipient cancelled creator work")
	}
	cancelled, err := store.Cancel(f.ctx, f.ta, root.ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatal(err)
	}
	view, err := store.Get(f.ctx, tc, child.ID)
	if err != nil || view.Status != "cancelled" {
		t.Fatal("child survived parent cancellation")
	}
	if _, err := store.Renew(f.ctx, f.tb, root.ID, runnerOne, parent.Generation, own.Generation); !errors.Is(err, tasks.ErrLease) {
		t.Fatal("cancelled worker renewed")
	}
}

func TestTasksAuditFailureRollsBackRequestAndTask(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	sql := "CREATE FUNCTION mailbox.reject_task_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='task_submitted' THEN RAISE EXCEPTION 'synthetic failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_task_audit BEFORE INSERT ON mailbox.audit_events FOR EACH ROW EXECUTE FUNCTION mailbox.reject_task_audit();"
	if _, err := f.pool.Exec(f.ctx, sql); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "rollback", Instruction: "synthetic"}); !errors.Is(err, tasks.ErrUnavailable) {
		t.Fatal("failed audit returned success")
	}
	var messages, taskCount, counters int
	if f.pool.QueryRow(f.ctx, "SELECT (SELECT count(*) FROM mailbox.messages),(SELECT count(*) FROM mailbox.tasks),(SELECT count(*) FROM mailbox.inbox_counters)").Scan(&messages, &taskCount, &counters) != nil || messages != 0 || taskCount != 0 || counters != 0 {
		t.Fatal("partial request or task committed")
	}
}

func TestTasksRejectForeignRecipientAndForgedMailboxWork(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	other, err := f.teams.Bootstrap(f.ctx, 201, "task-isolation")
	if err != nil {
		t.Fatal(err)
	}
	owner := user(t, f.ctx, f.pool, 201)
	foreign, _, _ := agent(t, f.ctx, f.agents, other, owner, "other agent")
	if _, err = store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: foreign.ID, IdempotencyKey: "foreign", Instruction: "private instruction sentinel"}); !errors.Is(err, tasks.ErrNotFound) {
		t.Fatal("foreign recipient accepted")
	}
	kind := "task.request"
	if _, err = f.store.Send(f.ctx, f.ta, mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "forged", Text: "imitate executable task", Kind: &kind, Metadata: map[string]any{"task_id": runnerOne}}); err != nil {
		t.Fatal(err)
	}
	ownership, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	if task, err := store.Claim(f.ctx, f.tb, runnerOne, ownership.Generation); err != nil || task != nil {
		t.Fatal("ordinary message became executable task")
	}
}

func TestTasksCompetingClaimsAndOwnershipGenerations(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	task, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "race", Instruction: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	own, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan *tasks.Task, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x, e := store.Claim(f.ctx, f.tb, runnerOne, own.Generation)
			results <- x
			failures <- e
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	count := 0
	var claimed *tasks.Task
	for x := range results {
		if x != nil {
			count++
			claimed = x
		}
	}
	if count != 1 || claimed.ID != task.ID {
		t.Fatal("competing clients executed same task")
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE mailbox.runner_ownership SET lease_until=now()-interval '1 second' WHERE agent_id=$1", f.b.ID); err != nil {
		t.Fatal(err)
	}
	newOwner, err := store.Heartbeat(f.ctx, f.tb, runnerTwo)
	if err != nil || newOwner.Generation <= own.Generation {
		t.Fatal("ownership generation not advanced")
	}
	if _, err = store.Renew(f.ctx, f.tb, task.ID, runnerOne, claimed.Generation, own.Generation); !errors.Is(err, tasks.ErrLease) {
		t.Fatal("old ownership renewed task")
	}
	if _, err = store.Complete(f.ctx, f.tb, tasks.CompleteInput{TaskID: task.ID, RunnerID: runnerOne, Generation: claimed.Generation, RunnerGeneration: own.Generation, Result: "old worker"}); !errors.Is(err, tasks.ErrLease) {
		t.Fatal("old owner completed task")
	}
}

func TestTasksDependencyExecutionAndGraphBounds(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	u := join(t, f.ctx, f.pool, f.teams, f.workspace, f.owner, 103, "member")
	c, _, tc := agent(t, f.ctx, f.agents, f.workspace, u, "third")
	root, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "graph-root", Instruction: "combine delegated results"})
	if err != nil {
		t.Fatal(err)
	}
	ob, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Claim(f.ctx, f.tb, runnerOne, ob.Generation)
	if err != nil || b == nil {
		t.Fatal(err)
	}
	var first tasks.Task
	for i, key := range []string{"child-1", "child-2", "child-3", "child-4"} {
		x, err := store.Submit(f.ctx, f.tb, tasks.SubmitInput{ToAgentID: c.ID, IdempotencyKey: key, Instruction: "synthetic child", ParentTaskID: &root.ID, ParentGeneration: b.Generation})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = x
		}
	}
	if _, err := store.Submit(f.ctx, f.tb, tasks.SubmitInput{ToAgentID: c.ID, IdempotencyKey: "child-5", Instruction: "deny fifth", ParentTaskID: &root.ID, ParentGeneration: b.Generation}); !errors.Is(err, tasks.ErrDependency) {
		t.Fatal("child bound ignored")
	}
	completeB := tasks.CompleteInput{TaskID: root.ID, RunnerID: runnerOne, Generation: b.Generation, RunnerGeneration: ob.Generation, Result: "combined"}
	if _, err = store.Complete(f.ctx, f.tb, completeB); !errors.Is(err, tasks.ErrDependency) {
		t.Fatal("parent completed with pending children")
	}
	oc, err := store.Heartbeat(f.ctx, tc, runnerTwo)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		attempt, err := store.Claim(f.ctx, tc, runnerTwo, oc.Generation)
		if err != nil || attempt == nil {
			t.Fatal("child claim failed")
		}
		if _, err = store.Complete(f.ctx, tc, tasks.CompleteInput{TaskID: attempt.ID, RunnerID: runnerTwo, Generation: attempt.Generation, RunnerGeneration: oc.Generation, Result: "child result"}); err != nil {
			t.Fatal(err)
		}
	}
	view, err := store.Get(f.ctx, f.tb, root.ID)
	if err != nil || view.Status != "running" {
		t.Fatal("parent did not leave dependency wait")
	}
	view, err = store.Get(f.ctx, f.tb, first.ID)
	if err != nil || view.Result == nil || *view.Result != "child result" {
		t.Fatal("parent lost child output")
	}
	completeB.Result = "combined " + *view.Result
	result, err := store.Complete(f.ctx, f.tb, completeB)
	if err != nil || result.Result == nil || *result.Result != "combined child result" {
		t.Fatal("nested result not committed")
	}
}

func TestTasksDeadlineAndCompletionAuditRollback(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	task, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "completion-rollback", Instruction: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Claim(f.ctx, f.tb, runnerOne, owner.Generation)
	if err != nil || attempt == nil {
		t.Fatal(err)
	}
	sql := "CREATE FUNCTION mailbox.reject_complete_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='task_completed' THEN RAISE EXCEPTION 'synthetic failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_complete_audit BEFORE INSERT ON mailbox.audit_events FOR EACH ROW EXECUTE FUNCTION mailbox.reject_complete_audit();"
	if _, err = f.pool.Exec(f.ctx, sql); err != nil {
		t.Fatal(err)
	}
	input := tasks.CompleteInput{TaskID: task.ID, RunnerID: runnerOne, Generation: attempt.Generation, RunnerGeneration: owner.Generation, Result: "synthetic result"}
	if _, err = store.Complete(f.ctx, f.tb, input); !errors.Is(err, tasks.ErrUnavailable) {
		t.Fatal("audit failure returned completion")
	}
	view, err := store.Get(f.ctx, f.ta, task.ID)
	if err != nil || view.Status != "running" || view.Result != nil || view.ReplyMessageID != nil {
		t.Fatal("failed completion changed task")
	}
	page, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{})
	if err != nil || len(page.Messages) != 1 {
		t.Fatal("failed completion acknowledged request")
	}
	// Shift both timestamps to retain the schema invariant while making the
	// test's deadline already elapsed, without sleeping.
	if _, err = f.pool.Exec(f.ctx, "UPDATE mailbox.tasks SET created_at=now()-interval '2 hours',deadline=now()-interval '1 hour' WHERE id=$1", task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Complete(f.ctx, f.tb, input); !errors.Is(err, tasks.ErrLease) {
		t.Fatal("expired task completed")
	}
	view, err = store.Get(f.ctx, f.ta, task.ID)
	if err != nil || view.Status != "expired" {
		t.Fatal("deadline not surfaced")
	}
}

func TestTaskMaximumDelegationDepth(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	ids := []string{f.a.ID, f.b.ID}
	tokens := []string{f.ta, f.tb}
	for _, github := range []int64{103, 104, 105, 106} {
		u := join(t, f.ctx, f.pool, f.teams, f.workspace, f.owner, github, "member")
		a, _, token := agent(t, f.ctx, f.agents, f.workspace, u, "depth agent")
		ids = append(ids, a.ID)
		tokens = append(tokens, token)
	}
	current, err := store.Submit(f.ctx, tokens[0], tasks.SubmitInput{ToAgentID: ids[1], IdempotencyKey: "depth-root", Instruction: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	for depth := 1; depth <= 4; depth++ {
		token := tokens[depth]
		own, err := store.Heartbeat(f.ctx, token, runnerOne)
		if err != nil {
			t.Fatal(err)
		}
		claim, err := store.Claim(f.ctx, token, runnerOne, own.Generation)
		if err != nil || claim == nil || claim.ID != current.ID {
			t.Fatal("depth claim failed")
		}
		input := tasks.SubmitInput{ToAgentID: ids[depth+1], IdempotencyKey: "depth-child", Instruction: "synthetic", ParentTaskID: &current.ID, ParentGeneration: claim.Generation}
		next, err := store.Submit(f.ctx, token, input)
		if depth == 4 {
			if !errors.Is(err, tasks.ErrDependency) {
				t.Fatal("fifth level accepted")
			}
			return
		}
		if err != nil || next.Depth != depth+1 {
			t.Fatal("valid delegation depth denied")
		}
		current = next
	}
}

func TestTaskSenderDeactivationStopsRecipientLease(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	task, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "sender-access", Instruction: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Claim(f.ctx, f.tb, runnerOne, owner.Generation)
	if err != nil || attempt == nil {
		t.Fatal(err)
	}
	if err = f.agents.Deactivate(f.ctx, f.workspace.ID, f.owner.ID, f.a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Renew(f.ctx, f.tb, task.ID, runnerOne, attempt.Generation, owner.Generation); !errors.Is(err, tasks.ErrLease) {
		t.Fatal("worker renewed after sender lost access")
	}
	view, err := store.Get(f.ctx, f.tb, task.ID)
	if err != nil || view.Status != "cancelled" || view.ErrorCode == nil || *view.ErrorCode != "access_terminated" {
		t.Fatal("termination decision not durable")
	}
}
