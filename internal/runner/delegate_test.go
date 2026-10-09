package runner

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/petarnenov/bot-space/internal/mcpclient"
	"github.com/petarnenov/bot-space/internal/tasks"
)

func TestDelegationKeyStableAndScopedByJob(t *testing.T) {
	input := DelegateInput{ToAgentID: "22222222-2222-4222-8222-222222222222", Instruction: "do work", TimeoutSeconds: 30}
	k1 := delegationKey("11111111-1111-4111-8111-111111111111", "", input)
	k2 := delegationKey("11111111-1111-4111-8111-111111111111", "", input)
	if k1 != k2 {
		t.Fatal("stable key changed across identical input")
	}
	if other := delegationKey("33333333-3333-4333-8333-333333333333", "", input); other == k1 {
		t.Fatal("job scoping missing from delegation key")
	}
	if explicit := delegationKey("11111111-1111-4111-8111-111111111111", "fixed", input); explicit == k1 {
		t.Fatal("explicit request key did not change identity")
	}
}

func TestAuthorizedJobRequiresCapabilityAndActiveExecution(t *testing.T) {
	jobID := "11111111-1111-4111-8111-111111111111"
	cap := "private-capability"
	s := &Supervisor{
		jobs:    map[string]*Job{jobID: &Job{ID: jobID, AgentID: "22222222-2222-4222-8222-222222222222", Capability: cap}},
		active:  map[string]context.CancelFunc{jobID: func() {}},
		clients: map[string]*mcpclient.Client{"22222222-2222-4222-8222-222222222222": &mcpclient.Client{}},
	}
	req := httptest.NewRequest("POST", "/jobs/"+jobID+"/delegate", nil)
	req.SetPathValue("id", jobID)
	req.Header.Set("X-Job-Capability", cap)
	if _, _, ok := s.authorizedJob(req); !ok {
		t.Fatal("authorized job was denied")
	}
	req.Header.Set("X-Job-Capability", "wrong")
	if _, _, ok := s.authorizedJob(req); ok {
		t.Fatal("wrong capability was accepted")
	}
	delete(s.active, jobID)
	req.Header.Set("X-Job-Capability", cap)
	if _, _, ok := s.authorizedJob(req); ok {
		t.Fatal("inactive job was accepted")
	}
}

func TestServeStartupMarksInterruptedRuntimeForReview(t *testing.T) {
	root := t.TempDir()
	project := root + "/project"
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Endpoint: "https://example.com/mcp",
		StateDir: root + "/state",
		Agents: []AgentConfig{{
			ID:             "22222222-2222-4222-8222-222222222222",
			TokenEnv:       "RUNNER_TEST_TOKEN",
			Provider:       "copilot",
			Project:        project,
			AllowedSenders: []string{"11111111-1111-4111-8111-111111111111"},
		}},
	}
	s, err := NewSupervisor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	jobID := "11111111-1111-4111-8111-111111111111"
	s.jobs[jobID] = &Job{
		ID:          jobID,
		AgentID:     cfg.Agents[0].ID,
		Instruction: "continue",
		Status:      "running",
		Deadline:    time.Now().Add(time.Hour),
		Capability:  "cap",
	}
	if err := saveJob(cfg.StateDir, s.jobs[jobID]); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Serve(ctx); err != nil {
		t.Fatal(err)
	}
	if s.jobs[jobID].Status != "requires_approval" || s.jobs[jobID].ErrorCode != "uncertain_interruption" {
		t.Fatal("running restart state was not fenced for review")
	}
}

func TestServeStartupKeepsDeferredContinuationOnExactSession(t *testing.T) {
	root := t.TempDir()
	project := root + "/project"
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Endpoint: "https://example.com/mcp",
		StateDir: root + "/state",
		Agents: []AgentConfig{{
			ID:             "22222222-2222-4222-8222-222222222222",
			TokenEnv:       "RUNNER_TEST_TOKEN",
			Provider:       "copilot",
			Project:        project,
			AllowedSenders: []string{"11111111-1111-4111-8111-111111111111"},
		}},
	}
	s, err := NewSupervisor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	jobID := "11111111-1111-4111-8111-111111111111"
	s.jobs[jobID] = &Job{
		ID:          jobID,
		AgentID:     cfg.Agents[0].ID,
		Instruction: "continue",
		Status:      "running",
		SessionID:   "33333333-3333-4333-8333-333333333333",
		Deadline:    time.Now().Add(time.Hour),
		Capability:  "cap",
		Dependencies: []Dependency{{
			Key: "dep1",
			Input: tasks.SubmitInput{
				ToAgentID:      "44444444-4444-4444-8444-444444444444",
				Instruction:    "nested",
				IdempotencyKey: "k",
			},
			Delivered: false,
		}},
	}
	if err := saveJob(cfg.StateDir, s.jobs[jobID]); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Serve(ctx); err != nil {
		t.Fatal(err)
	}
	if s.jobs[jobID].Status != "waiting_dependency" {
		t.Fatal("deferred continuation was not preserved")
	}
	if s.jobs[jobID].SessionID != "33333333-3333-4333-8333-333333333333" {
		t.Fatal("exact session mapping changed")
	}
}
