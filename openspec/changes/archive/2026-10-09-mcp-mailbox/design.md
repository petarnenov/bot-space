# Design

## Context

The first three changes are verified and archived. Agents have scoped credentials and transaction-bound authorization rechecks. The user approved the complete architecture and authorized sequential work. This change adds actual HTTP MCP and durable mailbox behavior; it must not substitute direct Go calls for the two-client HTTP proof.

Official documentation now identifies MCP `2026-07-28` as latest. The pinned Go SDK v1.8.0 supports this era and compatibility handling for older revisions. Source inspection confirms stateless HTTP, JSON responses, request-body bounds, cancellation propagation, and actual HTTP headers in `CallToolRequest.Extra.Header`.

## Goals / Non-Goals

**Goals:** five interoperable tools, committed offline-capable delivery, safe threads/replies, sender-scoped idempotency, own-inbox processing, stable concurrent pagination, and an executable two-process client example.

**Non-Goals:** agent activation/runners, server push, single-consumer claims/leases, exactly-once execution, full MCP OAuth, cross-workspace routing, automated deletion, paid resources, or deployment.

## Decisions

### Official transport and authentication

Use `mcp.NewStreamableHTTPHandler` with `Stateless: true`, `JSONResponse: true`, `MaxRequestBodyBytes: 1<<20`, and `PropagateRequestCancellation: true`. Keep SDK localhost protection enabled by default. A static server registers the five tools; every HTTP method is wrapped by existing uncached agent authentication and explicit Origin validation before dispatch. The existing global body/timeouts remain active. Configure SDK loggers to discard so protocol/debug logs cannot expose request bodies, cookies or bearer tokens.

Tool handlers take bearer credentials only from the SDK's transport-populated `Extra.Header`, never arguments or browser sessions. Each handler rechecks authorization in its database transaction. This avoids depending on SDK context-value propagation, and uses the documented header field while keeping transport/JSON-RPC implementation entirely in the SDK.

Use the SDK's low-level `Server.AddTool` API with explicit object schemas and typed, strict application argument decoding. That API explicitly assigns argument validation to the tool author. Reject unknown properties/nulls, invalid scalar types, UUIDs, field limits and enums with our stable bounded application error envelope. No custom protocol dispatcher, framing, negotiation, session implementation or client transport is added.

Results carry JSON in a standard text content block, allowing clients to parse the documented data model. Application errors use `IsError` plus `{"error":{"code":"...","message":"..."}}`; SDK protocol errors remain SDK-defined. Count the serialized tool-result wrapper toward 256 KiB, reserving a small margin for SDK-added completion metadata. This avoids duplicating large message data in both structured and text content. Response schemas are documented and tested as JSON text payloads.

### Configuration, origins, and signing

`CURSOR_SIGNING_KEY` is a 64-hex-character environment value representing 32 random bytes. Its presence enables the mailbox; invalid supplied values fail startup. Absent mailbox settings keep `/mcp` unavailable, independently of configured human login. `MCP_ALLOWED_ORIGINS` is an optional comma-separated exact-origin allowlist; include the configured browser public origin when present. Reject wildcard, null, malformed, duplicate and foreign Origin headers. Native clients without Origin remain authenticated normally. Origin permission does not authenticate browser cookies or grant team access.

HMAC-SHA256 cursors bind version/purpose, agent UUID, workspace UUID, filters, upper sequence, and last sequence (or last directory UUID). Validate signature and scope before querying. Limit cursor length to 2048 bytes. The signing key stays only in environment/memory; preserve it across redeploy for existing cursors. Changing it invalidates cursors but not messages or bearer credentials; start a fresh poll afterward.

Default SDK DNS-rebinding checks are retained. If a verified proxy deployment requires a different Host policy, introduce an explicit equivalent allowlist and test valid configured/proxy and foreign-host cases before disabling the SDK default. Do not claim Railway proxy behavior from a local direct connection.

### Message schema and storage

Add `0004_mailbox.sql` with participant-pair conversations, per-agent inbox counters, and messages. Each tenant-bearing relation uses workspace composite foreign keys to agents/conversations/messages. Public message JSON contains exactly the required identity/reference/content/timestamp fields; sequence, key and fingerprint are internal.

Message `metadata` uses PostgreSQL `json`, not `jsonb`: root-object and byte checks still apply, while lexical number precision and compact exponents survive without JSONB numeric conversion or expansion. Decode arguments with `UseNumber`; preserve metadata JSON in responses. Normalize numeric formatting for hashing using digit/exponent string operations with bounded arbitrary-size exponent integers, without expanding huge values or converting them to floating point. Encode compact JSON without HTML escaping for content; the MCP result wrapper supplies its own JSON escaping.

Text is 1–16 KiB valid UTF-8 excluding NUL (unsupported by PostgreSQL text). Metadata is an object whose compact encoded form is at most 8 KiB, maximum depth 8. Kind defaults to `message` and permits ASCII letters/digits plus `_.:-` up to 64 characters. Keys are 1–128 printable ASCII characters. UUID inputs normalize to lowercase. Limits are explicit in schemas/documentation and application validation.

### Recipient and reference authorization

Sender identity comes from `AuthenticateTx`. Resolve recipient only in the sender workspace; lock recipient owner membership, then agent, and recheck activation. Offline eligibility does not require a connected process or issued recipient token. Deactivation/removal cannot commit through held authorization locks; work already committed remains durable.

A conversation stores the lexically sorted participant UUID pair and workspace. Self-addressed messages are permitted, with an equal participant pair. New sends create a thread when no thread/reply is supplied. A supplied thread must exist in this workspace with the exact pair. A reply must reference the caller's incoming or own outgoing message within that pair/workspace; explicit thread must agree. Invalid/invisible references return `not_found` without loading or exposing message bodies. Reading still selects only incoming messages, even when outbound references are permitted for send.

### Sender-scoped idempotency

Serialize `(workspace,sender,key)` with a transaction advisory lock before reading the existing result or generating defaults. Normalize validated caller payload (recipient, kind, text, metadata, optional caller-supplied thread/reply), then SHA-256 fingerprint it. Object ordering, equivalent JSON number formats, UUID case, and explicit ordinary defaults are equivalent; meaningful text/kind/reference differences are not. A generated/inferred thread is not inserted into the original caller payload fingerprint.

An existing key with identical fingerprint returns the committed message before recipient/reference revalidation, including after recipient deactivation. Changed fingerprint conflicts. Token rotation preserves sender identity/key scope. Concurrent identical retries serialize and return one message. Persist key/fingerprint with the message indefinitely.

### Commit ordering and inbox pagination

Before inserting, transactionally insert/increment the recipient's counter row and retain its row lock until commit. The assigned sequence, message, conversation and fingerprint commit together. A later sequence cannot commit ahead of an earlier one; UUIDs, `bigserial`, and transaction-start timestamps do not provide that property and are not used for inbox ordering.

A fresh read captures the highest committed counter as an upper bound. Query own workspace/recipient sequences `>last` and `<=upper`, ascending, with live acknowledgement/thread/kind filters. Fetch at most requested count plus one and stop early when the complete encoded tool result would exceed the byte limit. Return a cursor after the last included message when more candidates remain. New inserts beyond the snapshot are visible on the next fresh poll. Acknowledgement changes may remove unacknowledged candidates during pagination; this is a delivery snapshot rather than a historical processing snapshot.

Directory pagination uses ascending agent UUIDs and an authenticated purpose-bound cursor. It lists current active eligible peers; membership/activation changes are live, and fresh listing picks up newly registered peers whose IDs precede an earlier cursor. No historical directory snapshot is promised.

### Acknowledgements, retention, and multiple clients

Read never acknowledges. Own acknowledgement updates `acknowledged_at=COALESCE(acknowledged_at,clock_timestamp())` and returns the same timestamp on retries. Sender/nonrecipient acknowledgement is denied without another inbox's content. Historical data remains accessible with acknowledged/all filters under current authorization.

Two clients with one identity can read the same unacknowledged message and race to acknowledge. There is no claim/lease, single-consumer delivery, or exactly-once business execution. Messages and idempotency records remain indefinitely; monitor capacity and approve a future retention change before deletion.

### Executable client and verification

Use the official SDK's client and `StreamableClientTransport` with separate HTTP clients/credentials, standalone SSE disabled for polling, and redirect forwarding disabled. A credential round tripper refuses another origin and clones headers, preventing accidental token forwarding. Client role processes use environment credentials, peer UUIDs, a shared demo run ID, and bounded one-second polling. Print IDs/status only, not tokens or message bodies.

Verify actual two independent SDK client sessions over HTTP, different human owners joined by invitation, send/read/ack/reply/read, revoked credentials, spoofed fields, foreign workspace/inbox/references, payload errors, byte limits, idempotency, metadata precision, concurrent commits, snapshot inserts and shared-identity reads. Restart the application while retaining PostgreSQL and reconnect a client to verify offline/persistence. Run the two client processes as an executable example, not just direct function tests. Keep the SDK responsible for both sides of the wire.

## Risks / Trade-offs

- [Per-inbox row serialization] → correctness at MVP scale; benchmark before changing delivery ordering.
- [Repeated recipient processing] → document idempotent business handling and no single-consumer guarantee.
- [Indefinite storage] → capacity monitoring and separate approved retention policy.
- [JSON text result overhead] → enforce count plus actual serialized-wrapper byte limits; support legacy content consumers.
- [Signing-key changes] → retain the environment secret across redeploy or begin fresh polls.
- [Proxy/DNS-rebinding behavior] → preserve SDK defaults until an equivalent tested Host policy is necessary.

## Migration Plan

Apply the additive migration with the existing explicit serialized command. Configure a random cursor key to enable MCP; configure exact allowed origins for browser-origin requests. Identity login remains independently configurable. Run all local/CI-equivalent checks before sync/archive. Actual Railway deployment and live external client registration remain separately authorized operational work.

## Official Sources

Checked on 2026-10-09: [latest MCP transport](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports), [tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools), and [Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0). Versioned local SDK source confirms StreamableHTTPOptions, low-level AddTool validation responsibility, ClientTransport HTTPClient/SSE options, and actual transport headers in RequestExtra. Supported protocol framing and compatibility remain SDK behavior.

Implementation evidence: the SDK v1.8.0 latest-protocol `extractName` helper decodes the request parameters through an intermediate float representation. Extreme exponents such as `1e1000000` fail its required-header extraction before tool dispatch. This is a clear SDK protocol rejection, not silently rounded persistence; tests verify both accepted large-integer precision over HTTP and exact compact-exponent domain storage. The dependency remains the official unmodified SDK, and the observed limitation is documented in `docs/mcp.md`.
