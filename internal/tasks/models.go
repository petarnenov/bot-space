// Package tasks provides durable agent work with fenced execution and results.
package tasks

import (
	"errors"
	"time"
)

const (
	MaxInstructionBytes   = 16384
	MaxResultBytes        = 65536
	DefaultTimeoutSeconds = 1800
	MaxTimeoutSeconds     = 7200
	LeaseDuration         = 60 * time.Second
)

var (
	ErrInvalid     = errors.New("invalid_argument")
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not_found")
	ErrConflict    = errors.New("idempotency_conflict")
	ErrLease       = errors.New("lease_conflict")
	ErrDependency  = errors.New("dependency_conflict")
	ErrUnavailable = errors.New("temporarily_unavailable")
)

type Task struct {
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspace_id"`
	FromAgentID      string     `json:"from_agent_id"`
	ToAgentID        string     `json:"to_agent_id"`
	ParentTaskID     *string    `json:"parent_task_id,omitempty"`
	RootTaskID       string     `json:"root_task_id"`
	Depth            int        `json:"depth"`
	Instruction      string     `json:"instruction"`
	RetrySafe        bool       `json:"retry_safe"`
	Status           string     `json:"status"`
	Deadline         time.Time  `json:"deadline"`
	CreatedAt        time.Time  `json:"created_at"`
	Generation       int64      `json:"generation"`
	LeaseUntil       *time.Time `json:"lease_until,omitempty"`
	Result           *string    `json:"result,omitempty"`
	ErrorCode        *string    `json:"error_code,omitempty"`
	RequestMessageID string     `json:"request_message_id"`
	ReplyMessageID   *string    `json:"reply_message_id,omitempty"`
	RunnerID         *string    `json:"-"`
	RunnerGeneration *int64     `json:"-"`
	Ancestors        []string   `json:"-"`
}

type SubmitInput struct {
	ToAgentID        string  `json:"to_agent_id"`
	IdempotencyKey   string  `json:"idempotency_key"`
	Instruction      string  `json:"instruction"`
	TimeoutSeconds   int     `json:"timeout_seconds,omitempty"`
	ParentTaskID     *string `json:"parent_task_id,omitempty"`
	RetrySafe        bool    `json:"retry_safe,omitempty"`
	ParentGeneration int64   `json:"parent_generation,omitempty"`
}

type Ownership struct {
	RunnerID   string    `json:"runner_id"`
	Generation int64     `json:"generation"`
	LeaseUntil time.Time `json:"lease_until"`
}

type CompleteInput struct {
	TaskID           string `json:"task_id"`
	RunnerID         string `json:"runner_id"`
	Generation       int64  `json:"generation"`
	RunnerGeneration int64  `json:"runner_generation"`
	Result           string `json:"result"`
	ErrorCode        string `json:"error_code,omitempty"`
}
