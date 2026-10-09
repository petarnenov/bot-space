package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExclusiveStateAndDurablePrivateJournal(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	lock, err := lockState(state)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := lockState(state); err == nil {
		other.Close()
		t.Fatal("duplicate supervisor acquired state")
	}
	if err = os.Mkdir(filepath.Join(state, "jobs"), 0700); err != nil {
		t.Fatal(err)
	}
	j := Job{ID: "11111111-1111-4111-8111-111111111111", AgentID: "22222222-2222-4222-8222-222222222222", Instruction: "private journal sentinel", Status: "waiting_dependency", SessionID: "33333333-3333-4333-8333-333333333333", Deadline: time.Now().Add(time.Hour), Capability: "local private capability"}
	if err = saveJob(state, &j); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(jobPath(state, j.ID))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("journal is not owner-only")
	}
	jobs, err := loadJobs(state)
	if err != nil || len(jobs) != 1 || jobs[0].Instruction != j.Instruction || jobs[0].SessionID != j.SessionID {
		t.Fatal("restart lost exact task/session journal")
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := lockState(state)
	if err != nil {
		t.Fatal("released lock stayed held")
	}
	next.Close()
}

func TestRunnerConfigFixedProjectsAndPermissions(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	c := Config{Endpoint: "https://example.com/mcp", StateDir: filepath.Join(root, "state"), Agents: []AgentConfig{{ID: "11111111-1111-4111-8111-111111111111", TokenEnv: "RUNNER_ALPHA_TOKEN", Provider: "copilot", Project: project, AllowedSenders: []string{"22222222-2222-4222-8222-222222222222"}}}}
	if err := c.Validate(); err != nil || c.Concurrency != 1 || c.Agents[0].Policy != "analysis" {
		t.Fatal("default policy/concurrency not applied")
	}
	bad := c
	bad.StateDir = filepath.Join(project, "private-state")
	if bad.Validate() == nil {
		t.Fatal("private state inside source accepted")
	}
	bad = c
	for _, concurrency := range []int{-1, 2, 8, 9} {
		bad.Concurrency = concurrency
		if bad.Validate() == nil {
			t.Fatalf("executor concurrency %d accepted", concurrency)
		}
	}
	bad = c
	bad.Endpoint = "http://outside.example/mcp"
	if bad.Validate() == nil {
		t.Fatal("insecure remote endpoint accepted")
	}
	bad = c
	bad.Agents = append([]AgentConfig{}, c.Agents...)
	bad.Agents[0].Policy = "yolo"
	if bad.Validate() == nil {
		t.Fatal("global bypass accepted")
	}
	configPath := filepath.Join(root, "runner.json")
	if err := atomicJSON(configPath, c); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(configPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(configPath); err == nil {
		t.Fatal("public-readable configuration accepted")
	}
}
