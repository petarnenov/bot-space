package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpclient"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/tasks"
)

type DelegateInput struct {
	ToAgentID      string `json:"to_agent_id"`
	Instruction    string `json:"instruction"`
	RequestKey     string `json:"request_key,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	RetrySafe      bool   `json:"retry_safe,omitempty"`
}

func delegationKey(jobID, requestKey string, input DelegateInput) string {
	encoded, _ := json.Marshal(input)
	hash := sha256.Sum256(encoded)
	identity := requestKey
	if identity == "" {
		identity = hex.EncodeToString(hash[:])
	}
	keyHash := sha256.Sum256([]byte(identity))
	return "runner:" + jobID + ":" + hex.EncodeToString(keyHash[:])
}

func (s *Supervisor) authorizedJob(r *http.Request) (*Job, *mcpclient.Client, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[r.PathValue("id")]
	if j == nil || s.active[j.ID] == nil || !security.Equal(j.Capability, r.Header.Get("X-Job-Capability")) {
		return nil, nil, false
	}
	client := s.clients[j.AgentID]
	if client == nil {
		return nil, nil, false
	}
	copy := *j
	copy.Dependencies = append([]Dependency{}, j.Dependencies...)
	return &copy, client, true
}

func (s *Supervisor) identityHTTP(rw http.ResponseWriter, r *http.Request) {
	_, client, ok := s.authorizedJob(r)
	if !ok {
		http.Error(rw, "Job access denied", 403)
		return
	}
	var identity agents.Principal
	if client.Call(r.Context(), "whoami", map[string]any{}, &identity) != nil {
		http.Error(rw, "Agent unavailable", 503)
		return
	}
	localJSON(rw, identity)
}

func (s *Supervisor) directoryHTTP(rw http.ResponseWriter, r *http.Request) {
	_, client, ok := s.authorizedJob(r)
	if !ok {
		http.Error(rw, "Job access denied", 403)
		return
	}
	var input mailbox.DirectoryInput
	if !readLocal(rw, r, &input) {
		return
	}
	var directory mailbox.DirectoryPage
	if client.Call(r.Context(), "list_agents", input, &directory) != nil {
		http.Error(rw, "Directory unavailable", 503)
		return
	}
	localJSON(rw, directory)
}

func (s *Supervisor) delegateHTTP(rw http.ResponseWriter, r *http.Request) {
	j, client, ok := s.authorizedJob(r)
	if !ok {
		http.Error(rw, "Job access denied", 403)
		return
	}
	var input DelegateInput
	if !readLocal(rw, r, &input) {
		return
	}
	if !security.ValidUUID(input.ToAgentID) || input.Instruction == "" || len(input.Instruction) > tasks.MaxInstructionBytes || input.TimeoutSeconds < 0 || input.TimeoutSeconds > tasks.MaxTimeoutSeconds || len(input.RequestKey) > 128 {
		http.Error(rw, "Invalid delegation", 400)
		return
	}
	input.ToAgentID = strings.ToLower(input.ToAgentID)
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = tasks.DefaultTimeoutSeconds
	}
	key := delegationKey(j.ID, input.RequestKey, input)
	dependencyIndex := -1
	s.mu.Lock()
	actual := s.jobs[j.ID]
	for i, d := range actual.Dependencies {
		if d.Key == key {
			if d.Input.ToAgentID != input.ToAgentID || d.Input.Instruction != input.Instruction || d.Input.RetrySafe != input.RetrySafe {
				s.mu.Unlock()
				http.Error(rw, "Delegation key conflict", 409)
				return
			}
			dependencyIndex = i
			break
		}
	}
	if dependencyIndex < 0 {
		if len(actual.Dependencies) >= 4 {
			s.mu.Unlock()
			http.Error(rw, "Delegation limit exceeded", 409)
			return
		}
		seconds := input.TimeoutSeconds
		remaining := int(time.Until(actual.Deadline).Seconds())
		if remaining < seconds {
			seconds = remaining
		}
		if seconds < 1 {
			s.mu.Unlock()
			http.Error(rw, "Job deadline exceeded", 409)
			return
		}
		taskInput := tasks.SubmitInput{ToAgentID: input.ToAgentID, Instruction: input.Instruction, IdempotencyKey: key, TimeoutSeconds: seconds, RetrySafe: input.RetrySafe}
		if actual.RemoteTask != nil {
			taskInput.ParentTaskID = &actual.RemoteTask.ID
			taskInput.ParentGeneration = actual.RemoteTask.Generation
		}
		actual.Dependencies = append(actual.Dependencies, Dependency{Key: key, Input: taskInput})
		dependencyIndex = len(actual.Dependencies) - 1
	}
	actual.Status = "waiting_dependency"
	if saveJob(s.Config.StateDir, actual) != nil {
		s.mu.Unlock()
		http.Error(rw, "Journal unavailable", 503)
		return
	}
	dependency := actual.Dependencies[dependencyIndex]
	s.mu.Unlock()
	if dependency.TaskID == "" {
		var task tasks.Task
		if err := client.Call(r.Context(), "submit_task", dependency.Input, &task); err != nil {
			var app *mcpclient.ApplicationError
			if errors.As(err, &app) && app.Code != "temporarily_unavailable" && app.Code != "rate_limited" {
				s.mu.Lock()
				actual := s.jobs[j.ID]
				actual.Dependencies[dependencyIndex].Delivered = true
				actual.Status = "running"
				_ = saveJob(s.Config.StateDir, actual)
				s.mu.Unlock()
			}
			http.Error(rw, "Task submission unavailable", 503)
			return
		}
		dependency.TaskID = task.ID
		s.mu.Lock()
		s.jobs[j.ID].Dependencies[dependencyIndex].TaskID = task.ID
		err := saveJob(s.Config.StateDir, s.jobs[j.ID])
		s.mu.Unlock()
		if err != nil {
			http.Error(rw, "Journal unavailable", 503)
			return
		}
	}
	ctx, cancel := context.WithDeadline(r.Context(), j.Deadline)
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		var result tasks.Task
		if err := client.Call(ctx, "get_task", map[string]any{"task_id": dependency.TaskID}, &result); err == nil && taskTerminal(result.Status) {
			s.mu.Lock()
			actual := s.jobs[j.ID]
			actual.Dependencies[dependencyIndex].Result = &result
			actual.Dependencies[dependencyIndex].Delivered = true
			actual.Status = "running"
			err := saveJob(s.Config.StateDir, actual)
			s.mu.Unlock()
			if err != nil {
				http.Error(rw, "Journal unavailable", 503)
				return
			}
			localJSON(rw, result)
			return
		}
		select {
		case <-ctx.Done():
			http.Error(rw, "Delegation waiting interrupted", 504)
			return
		case <-ticker.C:
		}
	}
}

func bridgeCall(ctx context.Context, state, job, capability, action string, input, destination any) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return errors.New("invalid bridge input")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://runner/jobs/"+job+"/"+action, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Job-Capability", capability)
	client := localClient(state)
	if action == "delegate" {
		client.Timeout = 2 * time.Hour
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("runner bridge unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("runner bridge request rejected")
	}
	if json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 2<<20)).Decode(destination) != nil {
		return errors.New("invalid bridge response")
	}
	return nil
}
