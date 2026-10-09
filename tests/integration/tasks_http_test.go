package integration

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpclient"
	"github.com/petarnenov/bot-space/internal/mcpserver"
	"github.com/petarnenov/bot-space/internal/tasks"
)

func TestTaskHTTPDelegationResultAndStrictSchemas(t *testing.T) {
	f := mailSetup(t)
	srv := httptest.NewServer(mcpserver.NewWithTasks(f.store, nil))
	defer srv.Close()
	a := native(t, f, srv, f.ta)
	b := native(t, f, srv, f.tb)
	third := join(t, f.ctx, f.pool, f.teams, f.workspace, f.owner, 103, "member")
	_, _, thirdToken := agent(t, f.ctx, f.agents, f.workspace, third, "unrelated")
	unrelated := native(t, f, srv, thirdToken)
	catalog, err := a.Session.ListTools(f.ctx, nil)
	if err != nil || len(catalog.Tools) != 12 {
		t.Fatal("additive task tools missing")
	}
	var task tasks.Task
	if err = a.Call(f.ctx, "submit_task", map[string]any{"to_agent_id": f.b.ID, "idempotency_key": "http-task", "instruction": "Return a synthetic analysis"}, &task); err != nil {
		t.Fatal(err)
	}
	var foreignResult any
	if err = unrelated.Call(f.ctx, "get_task", map[string]any{"task_id": task.ID}, &foreignResult); err == nil {
		t.Fatal("unrelated native client read private task")
	}
	var own tasks.Ownership
	if err = b.Call(f.ctx, "runner_heartbeat", map[string]any{"runner_id": runnerOne}, &own); err != nil {
		t.Fatal(err)
	}
	var lease struct {
		Task *tasks.Task `json:"task"`
	}
	if err = b.Call(f.ctx, "claim_task", map[string]any{"runner_id": runnerOne, "runner_generation": own.Generation}, &lease); err != nil || lease.Task == nil {
		t.Fatal("native claim failed")
	}
	for name, args := range map[string]map[string]any{
		"submit_task": {"to_agent_id": f.b.ID, "idempotency_key": "spoof", "instruction": "deny", "from_agent_id": f.b.ID},
		"get_task":    {"task_id": task.ID, "workspace_id": f.workspace.ID},
	} {
		var ignored any
		if err = a.Call(f.ctx, name, args, &ignored); err == nil {
			t.Fatal("unknown identity arguments accepted")
		}
	}
	var ignored any
	err = b.Call(f.ctx, "runner_heartbeat", map[string]any{"runner_id": runnerTwo}, &ignored)
	var app *mcpclient.ApplicationError
	if !errors.As(err, &app) || app.Code != "lease_conflict" {
		t.Fatal("unstable ownership error")
	}
	complete := map[string]any{"task_id": task.ID, "runner_id": runnerOne, "runner_generation": own.Generation, "generation": lease.Task.Generation, "result": "actual synthetic provider result"}
	missingResult := map[string]any{"task_id": task.ID, "runner_id": runnerOne, "runner_generation": own.Generation, "generation": lease.Task.Generation}
	if err = b.Call(f.ctx, "complete_task", missingResult, &ignored); err == nil {
		t.Fatal("missing required result accepted")
	}
	if err = b.Call(f.ctx, "complete_task", complete, &task); err != nil {
		t.Fatal(err)
	}
	if err = a.Call(f.ctx, "get_task", map[string]any{"task_id": task.ID}, &task); err != nil || task.Status != "completed" || task.Result == nil || *task.Result != "actual synthetic provider result" {
		t.Fatal("native result delivery failed")
	}
	encoded, err := mailbox.ToolResult(task)
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(encoded)
	if len(wire) > mailbox.MaxToolResultBytes {
		t.Fatal("task result exceeded MCP bound")
	}
}

func TestTaskResultWireOverflowRollsBackCompletion(t *testing.T) {
	f := mailSetup(t)
	store := tasks.New(f.store)
	task, err := store.Submit(f.ctx, f.ta, tasks.SubmitInput{ToAgentID: f.b.ID, IdempotencyKey: "overflow", Instruction: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	own, err := store.Heartbeat(f.ctx, f.tb, runnerOne)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := store.Claim(f.ctx, f.tb, runnerOne, own.Generation)
	if err != nil || claim == nil {
		t.Fatal(err)
	}
	_, err = store.Complete(f.ctx, f.tb, tasks.CompleteInput{TaskID: task.ID, RunnerID: runnerOne, RunnerGeneration: own.Generation, Generation: claim.Generation, Result: strings.Repeat("\x01", tasks.MaxResultBytes)})
	if !errors.Is(err, tasks.ErrInvalid) {
		t.Fatal("escaped oversized wire result accepted")
	}
	current, err := store.Get(f.ctx, f.ta, task.ID)
	if err != nil || current.Status != "running" || current.Result != nil || current.ReplyMessageID != nil {
		t.Fatal("overflow partially committed")
	}
	page, err := f.store.Read(f.ctx, f.tb, mailbox.ReadInput{})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].AcknowledgedAt != nil {
		t.Fatal("failed result acknowledged request")
	}
}
