# Delegated Task Model

Task support is implemented as an additive mailbox capability and guarded behind
`TASKS_ENABLED`. Existing mailbox deployments keep the original behavior when the
flag is disabled.

## Submission and Delivery

Tasks have an immutable ID, workspace, credential-derived sender, recipient,
root/parent correlation, instruction, deadline, status and attempt generation.
Submission commits a task and its task.request mailbox message in one transaction.
A message using that kind without the matching task row is ordinary data.

The client supplies an idempotency key scoped to its agent/workspace. Equivalent
recipient UUID case and default timeout normalize to one fingerprint. Identical
retries return the same task; changed payload returns idempotency_conflict.
Instruction size is at most 16384 UTF-8 bytes without NUL. Timeout defaults to
1800 seconds, with a maximum of 7200. Results are at most 65536 bytes; the mailbox
reply retains its existing smaller limit by carrying a summary and task reference.

## Execution and Results

One runner process owns an agent through a renewable 60-second ownership lease.
The recipient claims one execution attempt with a monotonically increasing
generation and renews it while the provider runs. A competing runner receives
lease_conflict. Completion checks both ownership and execution generations,
persists the actual result plus a correlated reply, then acknowledges the request.
Exact result-delivery retries are idempotent; changed results conflict.

Only current sender/recipient credentials can read a task. Claim/renew/complete
require the addressed recipient. Creator cancellation covers descendants.
Revoked credentials, inactive agents and removed owners cannot continue work.

## Deadlines, Dependencies and Retry

States include queued, running, waiting_dependency, completed, failed, cancelled,
expired, interrupted and requires_approval. Children inherit the root deadline.
An agent cannot delegate to itself or an ancestor; maximum depth is four with
at most four children per task. Parents cannot complete while children are active.

Started work with a lost lease becomes interrupted. It requeues only if explicitly
declared retry_safe at submission; model execution and local file changes are
not exactly-once. Cancellation/deadline reject late completion. The runner must
terminate the process tree when renewal is denied. Requests are not acknowledged
before durable completion, so recovery never confuses receipt with processing.

Runner-controlled provider/session behavior is documented in
[runner settings](runner-settings.md) and service operations are documented in
[runner packaging and service operations](runner-service.md).

## Remote Tool Contract

All tools require the current bearer identity and strict object arguments.
Unknown fields and null values are rejected. HTTP Origin/body/rate policy matches
the existing mailbox transport.

| Tool | Inputs |
| --- | --- |
| submit_task | to_agent_id, idempotency_key, instruction; optional timeout_seconds, retry_safe, parent_task_id and its parent_generation |
| get_task | task_id; only current sender/recipient may read |
| runner_heartbeat | runner_id; returns ownership generation and lease deadline |
| claim_task | runner_id, runner_generation; returns one task or task:null |
| renew_task | task_id, runner_id, runner_generation, generation |
| complete_task | task_id, runner_id, runner_generation, generation, result; optional bounded error_code |
| cancel_task | task_id; only creator cancels its graph |

Task status/result is read by get_task. Full results stay within the 64 KiB text
and 256 KiB serialized MCP result limits; excessive JSON escaping can make a
smaller result invalid. Oversize completion rolls back result, reply and ACK.
Stable task errors add lease_conflict and dependency_conflict to the existing
mailbox error vocabulary. There are twelve tools when tasks are enabled; existing
mailbox-only clients keep their original five tools when the feature is disabled.

## Participant task pages and controls

With browser identity enabled and `TASKS_ENABLED=true`, participant owners can use:

- `GET /workspaces/{workspaceID}/tasks` for participant-scoped list/status/presence
- `GET /workspaces/{workspaceID}/tasks/{taskID}` for task detail/instruction/result
- `POST /workspaces/{workspaceID}/tasks/{taskID}/cancel` for creator cancellation
- `POST /workspaces/{workspaceID}/tasks/{taskID}/review` with `action=retry` for explicit retry review

The pages enforce current participant ownership, CSRF+same-origin checks, escaped
external content rendering, and `Cache-Control: no-store` plus `Referrer-Policy:
no-referrer`. Unrelated administrators cannot view other participants' task bodies.
