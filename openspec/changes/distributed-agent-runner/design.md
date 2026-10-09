# Distributed Agent Runner Design

> Superseded by `architect-led-orchestration`. The six completed foundation tasks remain historical evidence; unchecked tasks are not completed and are reconciled into the new architecture.
## Context

See proposal.md for the requested workflow. The existing server uses Go, pgx, versioned SQL and official SDK Streamable HTTP with a 15-second native client timeout and 60-second server write timeout. Agents have credential-derived identities and owner-scoped management. Mailbox ACK is not an execution claim. The requestreply example constructs a synthetic reply without model execution.

The user selected one runner per physical machine with managed persistent sessions and requested Codex and Copilot clients for this change scope. Existing interactive applications remain independent; managed sessions are owned by this runner.

## Goals / Non-Goals

Provide A delegates to B, B executes locally, A automatically continues with the result, including provider substitution and restart recovery. Preserve private workspace/inbox boundaries.

Do not deploy provider processes to Railway, open inbound machine ports, share local projects automatically, promise exactly-once side effects, take over unrelated terminals or automatically treat all mailbox text as executable instructions. File transfer, shared checkouts and arbitrary attachment synchronization are separate work; each recipient uses its configured local checkout and returns text plus bounded references.

## Decisions

### Durable task service and additive tools

Add internal/tasks and a forward migration for tasks, attempts, runner ownership and safe audits. Tasks store root/parent IDs, true participants, key/fingerprint, instruction, deadline, status, generation, lease expiry and bounded result/error. Tenant composite foreign keys enforce references. Submission/completion create corresponding task.request/task.result messages transactionally through the existing domain/store patterns. Reserved kind alone never grants execution: the task row must identify the addressed agent.

Add remote submit_task, get_task, claim_task, renew_task, complete_task, cancel_task and runner_heartbeat with strict schemas. claim_task returns one eligible task for the authenticated recipient, not an arbitrary inbox selector. Every call uses current credential/membership checks and safe typed errors. Existing five mailbox tools remain compatible. Reply messages contain a bounded summary/task reference within existing mailbox limits; the full result is stored on the task and read through get_task.

Remote operations finish within existing HTTP/database deadlines. Long-polling the Railway SDK handler for model execution would exceed current client/server timeouts, so waiting belongs in the local bridge. No Redis or broker is required.

### One local supervisor and explicit policy

Add cmd/runner with serve, doctor, start-task, status and bridge modes. A local configuration lists agent ID/token environment reference, provider executable, fixed project path, allowed senders, model choice, permission profile and machine concurrency (default one, maximum eight). Secrets stay in environment/owner-only external configuration. Validate authentication without starting a model during doctor.

An OS lock on the private runner state directory prevents duplicate supervisors. Each configured agent also acquires server ownership so copying its token to another machine cannot create two workers. Poll every five seconds with jitter/backoff and honor Retry-After; renew execution/ownership leases every 20 seconds and heartbeat every 30 seconds. Mark presence offline after 90 seconds. Background lease work remains responsive while a provider waits for a delegated result.

Serialize turns by provider session and project path. Reject delegation ancestry involving the same agent and enforce depth four/four children plus root deadlines. Local unavailable execution slots remain queued; waiting leases keep renewing. Deadlines surface resource contention instead of waiting forever.

Default policy allows analysis. Writes and tool permissions require explicit operator configuration. The task cannot choose a local path, executable, command template, model account or permission bypass. A task requesting unavailable permission becomes requires_approval with a reason; operator continuation is explicit, not an automatic bypass.

### Provider adapters and session continuity

Use a small Go provider interface for capabilities, start/resume, normalized streaming events and process-tree cancellation. Pin and check supported CLI versions during implementation. Use exact recorded session IDs, never --last or globally most-recent continuation.

- Codex: documented non-interactive execution with JSON events and exact exec resume session ID; prompts go through stdin.
- GitHub Copilot CLI: documented programmatic JSONL and exact --session-id/explicit resume. Supply private task content through supported stdin or an owner-only input file under a locally approved read path, with a constant instruction referring to that file. Do not place task text in -p arguments. Verify installed capabilities and the actual session/result event schema before claiming support.

The local Copilot command currently resolves to a VS Code bootstrap wrapper, not a working CLI. Install the official supported CLI only during implementation, record its version, and test authentication/output/resume/MCP permissions with synthetic tasks. Copilot ACP is a preview alternative; this release does not depend on preview session loading or introduce an additional language runtime.

Provider accounts remain local and must permit non-interactive use. Model smoke tests use the operator's selected model/account rather than silently switching providers. Deterministic fake adapters in CI prove orchestration, not real model compatibility; separate real smoke checks are mandatory for Codex and Copilot in this change.

### Local MCP delegation and automatic sender continuation

The managed provider connects to an official SDK stdio bridge exposing delegate_task, whoami and list_agents. The bridge forwards over a protected local socket to the supervisor. A per-job local capability binds requests to the current agent/job; model parameters cannot supply source identity. Provider children do not inherit bot-space bearer tokens.

delegate_task accepts recipient, instruction and bounded timeout, derives a stable key from job/call identity, persists the outgoing dependency journal and submits remotely. It polls get_task with short HTTPS calls and returns the actual terminal result/error into the pending tool call. Configure provider MCP timeouts explicitly; Copilot documents a 30-second default that must not silently limit task duration.

If the provider nevertheless times out or the machine restarts, preserve the pending dependency and exact initiating session. When the result is durable, start a single serialized continuation turn containing the correlated task/result and original continuation objective. A local journal records intent, session, accepted result and continuation progress. Reconcile uncertain provider-turn interruption for owner review instead of claiming exactly-once continuation. Ordinary message receipt or unrelated result IDs never resume a session.

### Fencing and interruption

Task leases last 60 seconds. Current attempt generation must match completion; stale attempts cannot overwrite newer status/results. Retain a received result locally before attempting completion so transient delivery can retry idempotently within its valid lease. Commit terminal task/reply before ACK.

Loss of lease, cancellation, deadline or access termination stops the provider process tree. Started work with uncertain side effects becomes interrupted, not silently queued for another run. A submitted explicit retry-safe policy allows a new attempt; otherwise owner review authorizes retry. Queued tasks survive offline recipients until their deadline.

### Authorized visibility

Add workspace task pages listing only tasks whose sender/recipient agents belong to the authenticated human. Show instruction/result, lifecycle, last-seen and bounded failure reason. Owner/admin status alone does not grant task content. Use the existing browser protection/rendering pattern for cancel/review actions and escape returned text. Audit only identifiers and transitions.

## Risks / Trade-offs

- CLI/auth/version drift -> capability checks, pinned tested versions and separate real-provider evidence.
- Crash after filesystem change -> fenced results, interrupted state and explicit retry-safe policy; no exactly-once claim.
- Long local tool waits -> explicit provider timeouts and durable deferred continuation.
- Private local transcripts/input files -> owner-only storage outside tracked source, no body/token operational logs.
- Different local checkouts -> document prerequisite project paths and revision metadata; do not imply file synchronization.
- Slow/offline models or delegation loops -> deadlines, lease renewal and dependency bounds.

## Migration Plan

Apply additive task schema before enabling task tools; retain all existing mailbox data/behavior. Run real-PG authorization/lease/concurrency tests, deterministic two-runner process tests and separate real Codex/Copilot smoke checks. Package runner binaries/configuration for macOS and Linux with startup/shutdown instructions; auto-start remains an explicit local install action.

Deploy server task support to the already-authorized Railway project after local/CI verification. Configure and start one runner on each available physical machine with its own provider login, project path and agent token. Verify a real delegated task and automatic continuation across two machines; unavailable second-machine access must remain an explicit incomplete gate.

Rollback stops runners and disables task-tool mounting while retaining additive schema/data; do not edit applied migrations or delete task records. Keep AGENTS.md unchanged.

## Sources and Observed Scope

- [Codex non-interactive execution and resume](https://learn.chatgpt.com/docs/non-interactive-mode).
- [Copilot CLI command/session/MCP reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference).

Local Codex 0.162.0 help was inspected; no model turn was executed in planning. Copilot wrapper installation prompt was declined. Exact supported provider versions and machine installation evidence belong to implementation.
