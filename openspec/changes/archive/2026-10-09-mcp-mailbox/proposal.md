# Proposal

## Why

The verified identity/agent boundary now needs a durable mailbox exposed through standard remote MCP. Agents on separate machines must exchange messages privately, including after offline periods and restarts, without custom transport or execution guarantees.

## What Changes

- Mount official MCP Go SDK v1.8.0 Streamable HTTP at `/mcp`, with per-request bearer authentication, Origin checks, bounded requests/results, and stateless JSON responses.
- Add `whoami`, `list_agents`, `send_message`, `read_messages`, and `acknowledge_message` with explicit schemas and stable application errors.
- Persist messages, participant-pair threads, reply references, idempotency fingerprints, and per-inbox commit-ordered sequences in PostgreSQL.
- Add signed inbox pagination, own-inbox acknowledgement, repeat-delivery semantics, and indefinite MVP retention without automatic deletion.
- Add a two-process Go SDK request/reply example and real HTTP MCP end-to-end/integration checks with distinct users/credentials.
- Add explicit cursor signing configuration; preserve operations-only mode when mailbox/identity configuration is absent.

No automatic agent launching/wakeup, push/webhooks, single-consumer claims, exactly-once processing, full MCP OAuth, cross-workspace communication, paid provisioning, or deployment is introduced. Full management forms and final operational/client checks follow in change 5.

## Capabilities

### New Capabilities

- `mcp-mailbox-tools`: official remote transport, credential/origin boundary, five tool schemas, and bounded results/errors.
- `message-delivery`: committed storage, eligible recipients, participant-scoped references, idempotency, persistence, and retention.
- `inbox-processing`: own-inbox reads/acknowledgements, stable snapshot pagination, repeat delivery, and multiple-client behavior.

### Modified Capabilities

- `service-runtime`: enable configured authenticated MCP independently of browser login, while unconfigured product features remain unavailable.

## Impact

Add the official SDK dependency, `0004_mailbox.sql`, mailbox/MCP/client packages, cursor configuration, executable example, and integration tests. Reuse existing credentials, membership/agent transaction locks, HTTP body/timeouts, and PostgreSQL pool. Transport and JSON-RPC remain exclusively SDK responsibilities. Local examples contain synthetic data and no stored credentials.
