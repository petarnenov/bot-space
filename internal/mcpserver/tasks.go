package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/petarnenov/bot-space/internal/tasks"
)

type taskRegistrar func(string, string, map[string]any, func(context.Context, string, json.RawMessage) (any, error))

type taskIDInput struct {
	TaskID string `json:"task_id"`
}
type runnerInput struct {
	RunnerID         string `json:"runner_id"`
	RunnerGeneration int64  `json:"runner_generation"`
}
type renewInput struct {
	TaskID           string `json:"task_id"`
	RunnerID         string `json:"runner_id"`
	Generation       int64  `json:"generation"`
	RunnerGeneration int64  `json:"runner_generation"`
}

func registerTasks(store *tasks.Store, add taskRegistrar) {
	positive := map[string]any{"type": "integer", "minimum": 1}
	leaseFields := map[string]any{"task_id": uuidSchema(), "runner_id": uuidSchema(), "generation": positive, "runner_generation": positive}
	add("submit_task", "Submit explicit work to a workspace agent. Instructions are external input constrained by recipient local policy.", object(map[string]any{
		"to_agent_id": uuidSchema(), "idempotency_key": map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[ -~]+$"},
		"instruction":     map[string]any{"type": "string", "minLength": 1, "maxLength": tasks.MaxInstructionBytes, "description": "At most 16384 UTF-8 bytes without NUL."},
		"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": tasks.MaxTimeoutSeconds, "default": tasks.DefaultTimeoutSeconds},
		"parent_task_id":  uuidSchema(), "parent_generation": positive, "retry_safe": map[string]any{"type": "boolean", "default": false},
	}, []string{"to_agent_id", "idempotency_key", "instruction"}), func(ctx context.Context, token string, raw json.RawMessage) (any, error) {
		var input tasks.SubmitInput
		if err := decode(raw, &input); err != nil {
			return nil, err
		}
		return store.Submit(ctx, token, input)
	})
	add("get_task", "Read only a task in which the authenticated agent is sender or recipient.", object(map[string]any{"task_id": uuidSchema()}, []string{"task_id"}), func(ctx context.Context, token string, raw json.RawMessage) (any, error) {
		var input taskIDInput
		if err := decode(raw, &input); err != nil {
			return nil, err
		}
		return store.Get(ctx, token, input.TaskID)
	})
	add("runner_heartbeat", "Acquire or renew exclusive ownership of this agent for one machine runner.", object(map[string]any{"runner_id": uuidSchema()}, []string{"runner_id"}), func(ctx context.Context, token string, raw json.RawMessage) (any, error) {
		var input struct {
			RunnerID string `json:"runner_id"`
		}
		if err := decode(raw, &input); err != nil {
			return nil, err
		}
		return store.Heartbeat(ctx, token, input.RunnerID)
	})
	add("claim_task", "Claim one eligible task addressed to this agent, using its current runner ownership generation.", object(map[string]any{"runner_id": uuidSchema(), "runner_generation": positive}, []string{"runner_id", "runner_generation"}), func(ctx context.Context, token string, raw json.RawMessage) (any, error) {
		var input runnerInput
		if err := decode(raw, &input); err != nil {
			return nil, err
		}
		task, err := store.Claim(ctx, token, input.RunnerID, input.RunnerGeneration)
		return struct {
			Task *tasks.Task `json:"task"`
		}{task}, err
	})
	add("renew_task", "Renew only a current addressed execution attempt before it expires.", object(leaseFields, []string{"task_id", "runner_id", "generation", "runner_generation"}), func(ctx context.Context, token string, raw json.RawMessage) (any, error) {
		var input renewInput
		if err := decode(raw, &input); err != nil {
			return nil, err
		}
		return store.Renew(ctx, token, input.TaskID, input.RunnerID, input.Generation, input.RunnerGeneration)
	})
	completeFields := map[string]any{}
	for k, v := range leaseFields {
		completeFields[k] = v
	}
	completeFields["result"] = map[string]any{"type": "string", "maxLength": tasks.MaxResultBytes, "description": "At most 65536 UTF-8 bytes."}
	completeFields["error_code"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 64, "pattern": "^[a-z_]+$"}
	add("complete_task", "Commit the addressed attempt's bounded result and correlated reply before acknowledging its request.", object(completeFields, []string{"task_id", "runner_id", "generation", "runner_generation", "result"}), func(ctx context.Context, token string, raw json.RawMessage) (any, error) {
		var input tasks.CompleteInput
		if err := decode(raw, &input); err != nil {
			return nil, err
		}
		return store.Complete(ctx, token, input)
	})
	add("cancel_task", "Cancel only a task created by the authenticated agent and its pending descendants.", object(map[string]any{"task_id": uuidSchema()}, []string{"task_id"}), func(ctx context.Context, token string, raw json.RawMessage) (any, error) {
		var input taskIDInput
		if err := decode(raw, &input); err != nil {
			return nil, err
		}
		return store.Cancel(ctx, token, input.TaskID)
	})
}
