# Distributed Agent Runner

> Superseded by `architect-led-orchestration`. The six completed foundation tasks remain historical evidence; unchecked tasks are not completed and are reconciled into the new architecture.
## Why

The mailbox delivers messages but does not execute delegated work or return results into the sender's working session. Users need agents on separate physical machines to perform tasks automatically and let the initiating agent continue with the result.

## What Changes

- Add one supervised Go runner per machine, with locally configured agents, project directories, provider permissions, and persistent session mappings.
- Support Codex CLI, Claude Code, and GitHub Copilot CLI through explicit adapters; provider authentication remains on each machine.
- Add durable task submission, claims, renewable leases, fenced completion, cancellation, deadlines, and correlated results in PostgreSQL.
- Add a local MCP delegate_task bridge that submits work, waits locally, and returns results into the initiating model turn; remote HTTP operations remain short.
- Resume the exact initiating session after disconnection/restart using a durable continuation journal. Ordinary mailbox text remains data; explicit tasks drive execution.
- Add authorized task status/result views for participants' human owners, without granting administrators access to others' content.
- Preserve existing mailbox tools, own-inbox isolation, credential revocation, rate limits, and the Railway service's provider-neutral role.

## Capabilities

### New Capabilities

- task-delegation: Durable task lifecycle, ownership, worker fencing, correlated results and bounded dependency graphs.
- machine-runner: One local supervisor with provider adapters, session continuity, execution policy and restart recovery.
- task-observability: Participant-authorized task views, safe operational status and audit.

### Modified Capabilities

- mcp-mailbox-tools: Update the activation documentation contract to cover managed runners while retaining the executable mailbox demonstration.

## Impact

Add migrations, internal task services, additive remote MCP task tools, a runner executable/local MCP bridge, local configuration examples, browser task views and process-level integration tests. Reuse internal/agents authorization, mailbox delivery and the official SDK. No inbound machine ports, cloud model credentials, remote shell execution by Railway, global permission bypass, or automatic execution of arbitrary mailbox messages.

Existing interactive terminals and editor sessions are not taken over. This release uses runner-managed sessions. Provider installation/version/authentication must be verified during implementation; the local Copilot executable currently resolves to a VS Code wrapper that requests installation, so usable Copilot CLI execution is not yet verified. Live tests must identify their exact provider/account/machine coverage.
