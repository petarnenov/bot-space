// Package mailbox provides durable, tenant-scoped messaging and own-inbox processing.
package mailbox

import (
	"encoding/json"
	"errors"
	"time"
)

const MaxTextBytes = 16384
const MaxMetadataBytes = 8192
const MaxMetadataDepth = 8
const MaxToolResultBytes = 256 << 10
const ToolResultMargin = 1024
const DefaultPageSize = 50
const MaxPageSize = 100
const MaxCursorBytes = 2048
const OperationTimeout = 5 * time.Second

var (
	ErrInvalid     = errors.New("invalid_argument")
	ErrNotFound    = errors.New("not_found")
	ErrForbidden   = errors.New("forbidden")
	ErrConflict    = errors.New("idempotency_conflict")
	ErrUnavailable = errors.New("temporarily_unavailable")
)

type Message struct {
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	FromAgentID    string          `json:"from_agent_id"`
	ToAgentID      string          `json:"to_agent_id"`
	ThreadID       string          `json:"thread_id"`
	InReplyTo      *string         `json:"in_reply_to"`
	Kind           string          `json:"kind"`
	Text           string          `json:"text"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	AcknowledgedAt *time.Time      `json:"acknowledged_at"`
	Sequence       int64           `json:"-"`
}

type SendInput struct {
	ToAgentID      string         `json:"to_agent_id"`
	IdempotencyKey string         `json:"idempotency_key"`
	Text           string         `json:"text"`
	Kind           *string        `json:"kind,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	ThreadID       *string        `json:"thread_id,omitempty"`
	InReplyTo      *string        `json:"in_reply_to,omitempty"`
}

type ReadInput struct {
	Limit        *int    `json:"limit,omitempty"`
	Cursor       string  `json:"cursor,omitempty"`
	Acknowledged *string `json:"acknowledged,omitempty"`
	ThreadID     *string `json:"thread_id,omitempty"`
	Kind         *string `json:"kind,omitempty"`
}
type DirectoryInput struct {
	Limit  *int   `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type AckInput struct {
	MessageID string `json:"message_id"`
}
type Page struct {
	Messages   []Message `json:"messages"`
	NextCursor *string   `json:"next_cursor"`
}
type AgentInfo struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	OwnerUserID   string `json:"owner_user_id"`
	Name          string `json:"name"`
	OwnerUsername string `json:"owner_username"`
	OwnerGitHubID int64  `json:"owner_github_id"`
}
type DirectoryPage struct {
	Agents     []AgentInfo `json:"agents"`
	NextCursor *string     `json:"next_cursor"`
}
