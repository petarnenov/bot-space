package runner

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/mcpclient"
	"github.com/petarnenov/bot-space/internal/tasks"
)

func Doctor(ctx context.Context, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	for _, agent := range config.Agents {
		token := os.Getenv(agent.TokenEnv)
		if token == "" {
			return errors.New("agent token environment variable is unset")
		}
		client, err := mcpclient.Connect(ctx, config.Endpoint, token)
		if err != nil {
			return err
		}
		var identity agents.Principal
		err = client.Call(ctx, "whoami", map[string]any{}, &identity)
		if err == nil && identity.AgentID != agent.ID {
			err = errors.New("credential belongs to a different configured agent")
		}
		if err == nil {
			catalog, e := client.Session.ListTools(ctx, nil)
			err = e
			found := false
			if e == nil {
				for _, tool := range catalog.Tools {
					if tool.Name == "submit_task" {
						found = true
					}
				}
			}
			if err == nil && !found {
				err = errors.New("server task tools are disabled")
			}
		}
		client.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Supervisor) Serve(ctx context.Context) error {
	if err := s.listen(); err != nil {
		return err
	}
	failed := make(chan error, 1)
	go func() {
		if err := s.server.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failed <- errors.New("local runner control failed")
		}
	}()
	s.mu.Lock()
	for _, j := range s.jobs {
		if j.Status == "running" {
			pending := false
			delivered := false
			for _, d := range j.Dependencies {
				pending = pending || !d.Delivered
				delivered = delivered || d.Delivered
			}
			if pending && !delivered && j.SessionID != "" {
				j.Status = "waiting_dependency"
			} else {
				j.Status = "requires_approval"
				j.ErrorCode = "uncertain_interruption"
			}
			if err := saveJob(s.Config.StateDir, j); err != nil {
				s.mu.Unlock()
				return err
			}
		}
	}
	s.mu.Unlock()
	var workers sync.WaitGroup
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, a := range s.Config.Agents {
		a := a
		workers.Add(1)
		go func() { defer workers.Done(); s.pollAgent(workerCtx, a) }()
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cancel()
			s.mu.Lock()
			for _, stop := range s.active {
				stop()
			}
			s.mu.Unlock()
			workers.Wait()
			return nil
		case err := <-failed:
			return err
		case <-ticker.C:
			s.schedule(workerCtx)
		}
	}
}

func (s *Supervisor) pollAgent(ctx context.Context, a AgentConfig) {
	var client *mcpclient.Client
	defer func() {
		if client != nil {
			client.Close()
		}
	}()
	nextHeartbeat := time.Time{}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		wait := time.Duration(0)
		if client == nil {
			token := os.Getenv(a.TokenEnv)
			if token != "" {
				c, e := mcpclient.Connect(ctx, s.Config.Endpoint, token)
				if e == nil {
					var identity agents.Principal
					if c.Call(ctx, "whoami", map[string]any{}, &identity) == nil && identity.AgentID == a.ID {
						client = c
						s.mu.Lock()
						s.clients[a.ID] = client
						s.mu.Unlock()
					} else {
						c.Close()
						wait = jitteredBackoff(5 * time.Second)
					}
				} else {
					wait = jitteredBackoff(5 * time.Second)
				}
			}
		}
		if client != nil {
			now := time.Now()
			if !nextHeartbeat.After(now) {
				var owner tasks.Ownership
				err := client.Call(ctx, "runner_heartbeat", map[string]any{"runner_id": s.runnerID}, &owner)
				s.mu.Lock()
				if err == nil {
					s.ownership[a.ID] = owner
					nextHeartbeat = now.Add(20 * time.Second)
				} else {
					if fatalRemote(err) || !s.ownership[a.ID].LeaseUntil.After(now) {
						s.stopAgentLocked(a.ID)
					} else {
						wait = jitteredBackoff(retryBackoff(err))
					}
				}
				s.mu.Unlock()
			}
			s.refreshAgent(ctx, a, client)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		case <-ticker.C:
		}
	}
}

func fatalRemote(err error) bool {
	var app *mcpclient.ApplicationError
	return errors.As(err, &app) && (app.Code == "forbidden" || app.Code == "lease_conflict" || app.Code == "not_found")
}

func retryBackoff(err error) time.Duration {
	var app *mcpclient.ApplicationError
	if errors.As(err, &app) {
		switch app.Code {
		case "rate_limited":
			return 15 * time.Second
		case "temporarily_unavailable":
			return 5 * time.Second
		}
	}
	return 2 * time.Second
}

func jitteredBackoff(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	jitter := time.Duration(rand.Int63n(int64(base / 2)))
	return base + jitter
}

func (s *Supervisor) stopAgentLocked(id string) {
	delete(s.ownership, id)
	for job, stop := range s.active {
		if s.jobs[job].AgentID == id {
			stop()
		}
	}
}

func (s *Supervisor) refreshAgent(ctx context.Context, a AgentConfig, client *mcpclient.Client) {
	s.mu.Lock()
	owner := s.ownership[a.ID]
	var jobs []*Job
	busy := false
	for _, j := range s.jobs {
		if j.AgentID != a.ID {
			continue
		}
		if j.Status == "running" || j.Status == "waiting_dependency" || j.Status == "queued" || j.Status == "delivery_pending" {
			busy = true
		}
		if j.RemoteTask != nil && (j.Status == "running" || j.Status == "waiting_dependency" || j.Status == "delivery_pending") {
			clone := *j
			jobs = append(jobs, &clone)
		}
	}
	s.mu.Unlock()
	if !owner.LeaseUntil.After(time.Now()) {
		return
	}
	for _, j := range jobs {
		var renewed tasks.Task
		err := client.Call(ctx, "renew_task", map[string]any{"task_id": j.RemoteTask.ID, "runner_id": s.runnerID, "runner_generation": j.RunnerGeneration, "generation": j.RemoteTask.Generation}, &renewed)
		s.mu.Lock()
		if err != nil {
			if fatalRemote(err) || j.RemoteTask.LeaseUntil == nil || !j.RemoteTask.LeaseUntil.After(time.Now()) {
				if stop := s.active[j.ID]; stop != nil {
					stop()
				}
				s.jobs[j.ID].Status = "interrupted"
				s.jobs[j.ID].ErrorCode = "lease_lost"
				_ = saveJob(s.Config.StateDir, s.jobs[j.ID])
			}
		} else {
			s.jobs[j.ID].RemoteTask = &renewed
			_ = saveJob(s.Config.StateDir, s.jobs[j.ID])
		}
		s.mu.Unlock()
	}
	s.recoverDependencies(ctx, a, client)
	s.deliverResults(ctx, a, client)
	if !busy {
		var result struct {
			Task *tasks.Task `json:"task"`
		}
		err := client.Call(ctx, "claim_task", map[string]any{"runner_id": s.runnerID, "runner_generation": owner.Generation}, &result)
		if err == nil && result.Task != nil {
			task := result.Task
			capability, e := randomID()
			if e != nil {
				return
			}
			job := &Job{ID: task.ID, AgentID: a.ID, Instruction: task.Instruction, RemoteTask: task, Deadline: task.Deadline, Status: "queued", Capability: capability, RunnerGeneration: owner.Generation}
			if !a.permits(task.FromAgentID) {
				job.Status = "delivery_pending"
				job.ErrorCode = "sender_not_permitted"
			}
			s.mu.Lock()
			if saveJob(s.Config.StateDir, job) == nil {
				s.jobs[job.ID] = job
			}
			s.mu.Unlock()
		}
	}
}

func (s *Supervisor) schedule(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	projects := map[string]bool{}
	running := 0
	for id := range s.active {
		j := s.jobs[id]
		a, _ := s.agent(j.AgentID)
		projects[a.Project] = true
		if j.Status == "running" {
			running++
		}
	}
	for _, j := range s.jobs {
		if j.Status != "queued" || running >= s.Config.Concurrency {
			continue
		}
		a, ok := s.agent(j.AgentID)
		if !ok || projects[a.Project] || s.clients[j.AgentID] == nil || !s.ownership[j.AgentID].LeaseUntil.After(time.Now()) {
			continue
		}
		if !j.Deadline.After(time.Now()) {
			j.Status = "expired"
			j.ErrorCode = "deadline_exceeded"
			_ = saveJob(s.Config.StateDir, j)
			continue
		}
		execution, cancel := context.WithDeadline(ctx, j.Deadline)
		j.Status = "running"
		if saveJob(s.Config.StateDir, j) != nil {
			cancel()
			continue
		}
		s.active[j.ID] = cancel
		running++
		projects[a.Project] = true
		request := ProviderRequest{Agent: a, JobID: j.ID, StateDir: s.Config.StateDir, Capability: j.Capability, SessionID: j.SessionID, Instruction: j.Instruction}
		// Include already-returned dependencies in a new continuation turn,
		// while exact session resume retains the previous project context.
		if j.SessionID != "" && len(j.Dependencies) > 0 {
			body, _ := json.Marshal(j.Dependencies)
			request.Instruction = "Continue the original task in this exact session using these correlated delegated results. Do not resubmit finished work. Original objective:\n" + j.Instruction + "\nDelegated results:\n" + string(body)
		}
		id := j.ID
		request.OnSession = func(sid string) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			job := s.jobs[id]
			if job.SessionID != "" && job.SessionID != sid {
				return errors.New("provider session changed")
			}
			job.SessionID = sid
			return saveJob(s.Config.StateDir, job)
		}
		go s.execute(execution, cancel, id, request)
	}
}

func (s *Supervisor) execute(ctx context.Context, cancel context.CancelFunc, id string, request ProviderRequest) {
	defer cancel()
	result, err := s.executor(ctx, request)
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	delete(s.active, id)
	if j.Status == "interrupted" {
		_ = saveJob(s.Config.StateDir, j)
		return
	}
	pending := false
	for _, d := range j.Dependencies {
		pending = pending || !d.Delivered
	}
	if pending && j.SessionID != "" {
		j.Status = "waiting_dependency"
		j.Result = ""
		j.ErrorCode = ""
	} else {
		j.Result = result.Text
		j.ErrorCode = result.ErrorCode
		if err != nil && j.ErrorCode == "" {
			j.ErrorCode = "provider_failed"
		}
		if j.RemoteTask != nil {
			j.Status = "delivery_pending"
		} else if j.ErrorCode != "" {
			j.Status = "failed"
		} else {
			j.Status = "completed"
		}
	}
	_ = saveJob(s.Config.StateDir, j)
}

func (s *Supervisor) deliverResults(ctx context.Context, a AgentConfig, client *mcpclient.Client) {
	s.mu.Lock()
	var pending []Job
	for _, j := range s.jobs {
		if j.AgentID == a.ID && j.Status == "delivery_pending" {
			pending = append(pending, *j)
		}
	}
	s.mu.Unlock()
	for _, j := range pending {
		var result tasks.Task
		err := client.Call(ctx, "complete_task", tasks.CompleteInput{TaskID: j.RemoteTask.ID, RunnerID: s.runnerID, RunnerGeneration: j.RunnerGeneration, Generation: j.RemoteTask.Generation, Result: j.Result, ErrorCode: j.ErrorCode}, &result)
		s.mu.Lock()
		if err == nil {
			s.jobs[j.ID].Status = result.Status
			s.jobs[j.ID].RemoteTask = &result
			_ = saveJob(s.Config.StateDir, s.jobs[j.ID])
		} else if fatalRemote(err) {
			s.jobs[j.ID].Status = "interrupted"
			s.jobs[j.ID].ErrorCode = "delivery_lease_lost"
			_ = saveJob(s.Config.StateDir, s.jobs[j.ID])
		}
		s.mu.Unlock()
	}
}

func (s *Supervisor) recoverDependencies(ctx context.Context, a AgentConfig, client *mcpclient.Client) {
	s.mu.Lock()
	var pending []Job
	for id, j := range s.jobs {
		if j.AgentID == a.ID && j.Status == "waiting_dependency" && s.active[id] == nil {
			copy := *j
			copy.Dependencies = append([]Dependency{}, j.Dependencies...)
			pending = append(pending, copy)
		}
	}
	s.mu.Unlock()
	for _, j := range pending {
		ready := true
		for i, d := range j.Dependencies {
			if d.TaskID == "" {
				ready = false
				continue
			}
			var task tasks.Task
			if client.Call(ctx, "get_task", map[string]any{"task_id": d.TaskID}, &task) != nil || !taskTerminal(task.Status) {
				ready = false
				continue
			}
			j.Dependencies[i].Result = &task
		}
		if ready {
			s.mu.Lock()
			actual := s.jobs[j.ID]
			actual.Dependencies = j.Dependencies
			actual.Status = "queued"
			_ = saveJob(s.Config.StateDir, actual)
			s.mu.Unlock()
		}
	}
}

func taskTerminal(status string) bool {
	return status != "queued" && status != "running" && status != "waiting_dependency"
}

// WriteConfig is used by local setup and tests; it never embeds a credential.
func WriteConfig(path string, config Config) error {
	if !filepath.IsAbs(path) || strings.TrimSpace(path) == "" {
		return errors.New("configuration path must be absolute")
	}
	return atomicJSON(path, config)
}
