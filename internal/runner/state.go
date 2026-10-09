package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/tasks"
)

type Job struct {
	ID               string       `json:"id"`
	AgentID          string       `json:"agent_id"`
	Instruction      string       `json:"instruction"`
	Status           string       `json:"status"`
	SessionID        string       `json:"session_id,omitempty"`
	Deadline         time.Time    `json:"deadline"`
	RemoteTask       *tasks.Task  `json:"remote_task,omitempty"`
	Result           string       `json:"result,omitempty"`
	ErrorCode        string       `json:"error_code,omitempty"`
	Dependencies     []Dependency `json:"dependencies,omitempty"`
	Capability       string       `json:"capability"`
	RunnerGeneration int64        `json:"runner_generation,omitempty"`
}
type Dependency struct {
	Key       string            `json:"key"`
	Input     tasks.SubmitInput `json:"input"`
	TaskID    string            `json:"task_id,omitempty"`
	Result    *tasks.Task       `json:"result,omitempty"`
	Delivered bool              `json:"delivered"`
}

func atomicJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 2<<20 {
		return errors.New("runner state is invalid or too large")
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return errors.New("runner state write unavailable")
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(raw)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil || closeErr != nil {
		return errors.New("runner state write failed")
	}
	if os.Rename(name, path) != nil {
		return errors.New("runner state replace failed")
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return errors.New("runner state directory unavailable")
	}
	defer dir.Close()
	if dir.Sync() != nil {
		return errors.New("runner state directory sync failed")
	}
	return nil
}

func readJSON(path string, dest any) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 2<<20 {
		return errors.New("runner state file must be bounded, regular and owner-only")
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, dest) != nil {
		return errors.New("runner state file unreadable")
	}
	return nil
}

func jobPath(state, id string) string { return filepath.Join(state, "jobs", id+".json") }
func saveJob(state string, j *Job) error {
	if !security.ValidUUID(j.ID) {
		return errors.New("invalid local job identifier")
	}
	return atomicJSON(jobPath(state, j.ID), j)
}

func loadJobs(state string) ([]Job, error) {
	entries, err := os.ReadDir(filepath.Join(state, "jobs"))
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var j Job
		if err := readJSON(filepath.Join(state, "jobs", entry.Name()), &j); err != nil {
			return nil, err
		}
		if !security.ValidUUID(j.ID) || entry.Name() != j.ID+".json" {
			return nil, errors.New("invalid local job identifier")
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}
