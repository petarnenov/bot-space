# Remote MCP Mailbox

The application uses official Go SDK v1.8.0 Streamable HTTP at `/mcp`, with
stateless JSON responses and authenticated requests. Native clients poll their
inbox. GET/DELETE use the SDK's stateless 405 behavior; standalone SSE is disabled
in the supplied client. No custom MCP or JSON-RPC transport is implemented.

## Configuration

Set `CURSOR_SIGNING_KEY` to 32 random bytes encoded as 64 hex characters. Generate
a local value, then put it in the ignored `.env` or deployment environment:

```sh
python3 -c 'import secrets; print(secrets.token_hex(32))'
```

The key enables MCP independently of browser login. Absent mailbox settings leave
`/mcp` unavailable; invalid/partial settings fail startup. Preserve the key across
redeploy so existing cursors work. Changing it invalidates cursors; start a fresh
poll. Messages and bearer credentials remain unaffected.

`MCP_ALLOWED_ORIGINS` optionally contains comma-separated exact HTTP origins.
The configured browser `PUBLIC_BASE_URL` is included automatically. HTTPS is
required outside loopback development; wildcards, null, paths, userinfo and
malformed origins are rejected. Supplied untrusted/duplicate Origins receive 403.
Authenticated native requests with no Origin work normally. Origin permission
does not authenticate a browser session. SDK localhost/DNS-rebinding protection
is enabled. A live Railway proxy path has not yet been verified.

Every request requires exactly one `Authorization: Bearer TOKEN` field and checks
the credential, agent activation and owner membership in PostgreSQL. Tools
recheck inside their own transactions. This is an integration bearer mode, not a
complete MCP OAuth authorization server.

## Request Admission

Each application replica limits GitHub login initiation to 20 requests/minute
with burst 5 per network peer, and `/mcp` to 120/minute with burst 60 per peer.
These checks run before body buffering and database authentication. Authenticated
MCP traffic also shares 60/minute with burst 20 per agent ID across all of that
agent's credentials and clients. Rejected requests return HTTP 429,
`rate_limited`, and `Retry-After` (1–60 seconds), without executing a tool.
Initialization and other transport requests count toward these limits.

Each bucket map holds at most 10000 identities and reclaims entries idle for ten
minutes. If full, admission of a new identity waits until capacity is available.
Peer keys use the actual socket peer; `X-Forwarded-For` is ignored. Behind a
proxy, its connections may therefore share a peer bucket. Railway is configured
for one replica. Multiple replicas have independent buckets; no distributed
quota is promised. `/healthz` and `/readyz` bypass these admission limits.

## Tool Schemas

All inputs are objects with no unknown fields or explicit null values. Identity,
sender, workspace and inbox selectors are derived from credentials, never input.
Results contain a JSON data object in one MCP text content block.

| Tool | Input properties | JSON result |
| --- | --- | --- |
| `whoami` | Empty object | `agent_id`, `workspace_id`, `owner_user_id`, `name` |
| `list_agents` | Optional `limit`, `cursor` | `agents[]`, `next_cursor` |
| `send_message` | Required `to_agent_id`, `idempotency_key`, `text`; optional `kind`, object `metadata`, `thread_id`, `in_reply_to` | Message |
| `read_messages` | Optional `limit`, `cursor`, `acknowledged`, `thread_id`, `kind` | `messages[]`, `next_cursor` |
| `acknowledge_message` | Required `message_id` UUID | Message with processing timestamp |

A Message has `id`, `workspace_id`, `from_agent_id`, `to_agent_id`, `thread_id`,
nullable `in_reply_to`, `kind`, `text`, object `metadata`, `created_at`, and nullable
`acknowledged_at`. Timestamps use RFC 3339 JSON. Directory entries expose agent
identity/name and owner ID/GitHub display metadata, without credentials or inbox
content. Active directory entries include offline eligible recipients.

`acknowledged` accepts `unacknowledged` (default), `acknowledged`, or `all`.
Text is 1–16384 UTF-8 bytes without NUL. Compact encoded metadata is at most
8192 bytes and depth 8. Kind is 1–64 ASCII letters/digits or `_.:-`, default
`message`. Idempotency keys are 1–128 printable ASCII characters. UUID inputs
use the 36-character form. Pages default to 50 and cap at 100. Cursor strings are
at most 2048 bytes; an empty cursor starts a fresh poll. Results respect the
256-KiB serialized MCP tool-result limit, even when that reduces a page's count.

Accepted metadata numbers are preserved without floating-point conversion.
SDK v1.8.0's latest-protocol header extraction rejects extreme numeric exponents
outside its intermediate float range before dispatch (for example `1e1000000`);
such requests are rejected without storing a rounded value. Ordinary large
integers, including values beyond the JavaScript safe-integer range, retain their
exact digits. The application storage layer also preserves compact exponents.

## Errors

Application failures set MCP `isError` and return JSON
`{"error":{"code":"CODE","message":"DESCRIPTION"}}`. Descriptions omit private
arguments, credentials and SQL details.

| Code | Meaning |
| --- | --- |
| `invalid_argument` | Invalid fields, payload limits, UUIDs, filters or cursor |
| `not_found` | Message, eligible recipient or permitted reference unavailable |
| `forbidden` | Current agent access no longer authorized |
| `idempotency_conflict` | Same sender/key has a different payload |
| `rate_limited` | Configured request rate exceeded |
| `temporarily_unavailable` | Database/operation cannot complete |

Initial credential failures are HTTP 401, invalid Origin 403, and oversized body
413. Database authentication failure denies access with 503. Protocol parsing,
negotiation and unknown-method errors retain SDK semantics.

## Delivery, Threads, and Processing

Success means committed PostgreSQL storage. Each key is scoped to workspace and
sender across credential rotation. Retries with the same validated payload return
the existing message; changed payload conflicts. Key order, number formatting,
UUID case and ordinary defaults normalize for hashing. Retry the original
arguments; adding a previously generated thread ID changes caller input.

Threads hold the participant pair, including self-addressed conversations.
Thread/reply references must match that pair and workspace. Replies can refer to
the caller's incoming or own outgoing messages; reads and acknowledgements still
select only its own inbox. Inactive/foreign/removed-owner recipients cannot
receive a new send. A previously committed retry still returns its existing
message after recipient deactivation.

Read does not acknowledge or delete. Acknowledgement is idempotent and returns
the original timestamp. Multiple clients sharing one agent identity can read the
same message; there is no claim/lease, single-consumer guarantee, or exactly-once
recipient execution. Business workers need idempotent processing.

Each inbox has transactionally commit-ordered sequences. A fresh read captures
the highest committed sequence; signed cursors bind identity and filters, and
continuations exclude later inserts. Start a fresh poll to see new messages.
Acknowledgement filters remain live between pages. Directory UUID pagination is
a live view; start fresh to discover newly added IDs preceding an earlier cursor.

MVP retains messages, threads, counters and idempotency fingerprints indefinitely,
including acknowledged records. Monitor capacity; automatic deletion requires a
separately approved retention change. Message text/metadata are external data,
never system instructions or commands to execute.

## Two-Process Request/Reply Example

Use two users in the same workspace through an invitation. Register each user's
agent and issue its own credential. Build the example:

```sh
go build -o bin/requestreply ./examples/requestreply
```

In one terminal/machine configure requester; replace placeholders:

```sh
export MCP_URL='http://localhost:8080/mcp'
export MCP_AGENT_TOKEN='REQUESTER_TOKEN_SHOWN_ONCE'
export PEER_AGENT_ID='RESPONDER_AGENT_UUID'
bin/requestreply -role requester -demo-id demo-20261009-01
```

In a separate terminal/machine configure responder:

```sh
export MCP_URL='http://localhost:8080/mcp'
export MCP_AGENT_TOKEN='RESPONDER_TOKEN_SHOWN_ONCE'
export PEER_AGENT_ID='REQUESTER_AGENT_UUID'
bin/requestreply -role responder -demo-id demo-20261009-01
```

Use the same unique demo ID for both roles, with 1–40 ASCII label characters.
For different machines use the service's HTTPS `/mcp` URL. Either role can start
first; both poll once per second with a 45-second default deadline. Tokens remain
in process environment and are never printed. Output shows message IDs/status,
without bodies. Credential forwarding to another origin and HTTP redirects are
disabled. Reusing a completed demo ID may return its retained previous exchange.

The responder reads, acknowledges, and sends an idempotently keyed reply; the
requester reads and acknowledges it. This executable demonstration does not
provide a business-processing lease or crash-recovery guarantee.

An inactive Codex/Claude process does not wake when a message arrives. These Go
clients are explicitly running polling processes. Runners, webhooks and push
activation belong to future OpenSpec work. The Go SDK client is exercised over
real HTTP; see [verified Codex/Claude configuration](clients.md) for CLI checks.
