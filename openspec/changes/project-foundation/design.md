# Design

## Context

The working directory was empty and had no Git metadata, toolchain configuration, or source. `AGENTS.md` was created under the initial contributor-guide request and is preserved unchanged. OpenSpec 1.14.1 is initialized for Codex with the `spec-driven` schema. See `proposal.md` for motivation and scope.

Observed local tools: Go 1.21.4, Node 24.21.0, npm 11.19.0, Git 2.33.0, Docker Engine/CLI 29.5.2, Compose 5.1.3. Docker responds to `docker info`. No application, integration, or deployment checks have run.

## Goals / Non-Goals

**Goals:** implement the three foundation capability contracts after approval; keep persistent state external to the application; establish a privacy-preserving architecture for the complete MVP.

**Non-Goals:** foundation does not expose MCP tools, login, management actions, or message delivery. The full MVP has no cross-workspace messaging, automatic agent activation, delivery push, distributed broker, public team signup, or full MCP OAuth authorization server.

The following MVP decisions are an architectural baseline for review. Later changes must define their exact schemas and acceptance scenarios before implementation; foundation approval does not count as implementing these capabilities.

## Decisions

### 1. Single Go application and dependency baseline

Use Go 1.27.2, official `github.com/modelcontextprotocol/go-sdk` v1.8.0, `github.com/jackc/pgx/v5` v5.11.0, and PostgreSQL 18.6. These stable versions were checked against official release sources on 2026-10-08. SDK v1.8.0 requires Go 1.25.0 or newer. Upgrade the project toolchain through a verified Go distribution or the pinned build container; do not replace the machine's global Go installation without need.

Use `net/http`, `html/template`, pgx pooling, and embedded templates/static files. Pin modules in `go.mod`/`go.sum` when they are introduced; do not create a fake dependency lock during planning. Pin container digests after verifying published manifests. Keep OpenSpec 1.14.1 as development/CI tooling with Node 24.21.0; Node is not an application runtime.

Proposed layout:

```text
cmd/mailbox/           serve, migrate, later bootstrap-owner subcommands
internal/config/       validated environment configuration
internal/database/     pool, migrations, repositories
internal/httpserver/   routing, health, middleware, shutdown
internal/identity/     later GitHub OAuth and browser sessions
internal/workspaces/   later membership, invitations, role policy
internal/agents/       later ownership and credential lifecycle
internal/mailbox/      later durable messaging and authorization
internal/mcpserver/    later official SDK tool registration
internal/web/          later embedded server-rendered pages and assets
migrations/            embedded, numbered forward-only SQL
examples/requestreply/ later two separately credentialed MCP clients
tests/integration/     real PostgreSQL and HTTP integration checks
openspec/              proposals, delta specs, completed main specs
```

A separate SPA or microservices would add deployment and authentication boundaries without an MVP need. No Redis or Kafka is required for a PostgreSQL-backed polling mailbox.

### 2. Foundation HTTP and configuration

`mailbox serve` validates `DATABASE_URL`, `PORT`, and pool settings before opening the listener. Readiness combines a two-second database deadline with migration version/checksum validation. Failure to reach PostgreSQL produces readiness 503 while liveness remains 200. Do not log raw connection strings or driver errors containing SQL/values.

Implement the limits in `service-runtime`. Future MCP polling uses JSON responses rather than unbounded SSE streams, so the write timeout remains appropriate. Compose publishes the application only to host loopback (`127.0.0.1:8080`); the container still listens on `0.0.0.0:$PORT` as Railway requires.

On signals, mark draining, stop accepting requests, drain for 20 seconds, cancel remaining work, and close the pool. Log route names and operational outcomes rather than raw URLs, request headers, bodies, or query strings.

### 3. PostgreSQL and migration strategy

Use an explicit migration subcommand, an independent connection with a PostgreSQL advisory lock, and one transaction per migration. Store ordered migration versions and SHA-256 checksums in `public.mailbox_schema_migrations`, with all references schema-qualified so the role-dependent PostgreSQL search path cannot shadow it. The total command deadline is five minutes. Reject changed or unknown applied migrations. Foundation introduces only the ledger and operational schema needed for its requirements; domain tables arrive in their own changes.

An explicit pre-deploy command gives migration failure a clear deployment boundary. Automatic migrations in every serving process would complicate concurrent startup. Parameterize values in all SQL; embed trusted migration SQL. Use a bounded pgx pool, default maximum ten connections; tune it against the production database connection budget before scaling.

Schema verification is deliberately strict: an older binary will be unready against an unknown newer schema. This does not promise zero downtime during schema rollout. Future compatibility windows require a separately specified migration/readiness policy. Never perform destructive rollback automatically; prefer forward repairs and an independently verified backup restore.

### 4. Human identity and workspace administration (change 2)

GitHub OAuth web flow uses random, single-use, ten-minute `state`, PKCE S256, an exact configured callback URL, and a same-origin local return path. Retrieve `/user`, key identity by its immutable numeric `id`, and discard access/refresh tokens after login. Request no repository or email scope; do not depend on username stability. Mock the provider in integration tests; production endpoints remain fixed to GitHub.

Store browser sessions in PostgreSQL; send only random opaque session IDs in Secure, HttpOnly, SameSite=Lax cookies in production. Rotate session IDs on login, enforce an eight-hour absolute lifetime and 30-minute idle lifetime, and invalidate them on CSRF-protected POST logout. Persist OAuth attempt state and verifier in a short-lived server-side record; never log callback query parameters. All browser mutations require server-side authorization, a session-bound CSRF token, and valid same-origin context.

`mailbox bootstrap-owner --github-user-id <id> --workspace <slug>` creates or locates the identity by immutable ID, creates the workspace, assigns its first owner, and emits an audit event transactionally. It is an administrative CLI command requiring database access, with no public bootstrap HTTP route. Repeated matching requests are idempotent; conflicting existing ownership is refused. Login alone creates no membership.

Invitations specify the target GitHub numeric ID, a member/admin role, a random secret shown once, and a proposed 48-hour expiry. Store only its hash. Acceptance atomically checks target identity, expiry, cancellation, and unused status. Forwarding a link cannot change the target. Owner/admin can invite and remove eligible members; admins cannot remove owners or grant ownership. Owners can grant owner role. Serialize role changes under a workspace lock to preserve at least one owner under concurrent changes. Personal workspace self-service is outside this MVP.

### 5. Agent credentials and authorization (change 3)

An agent belongs to one membership and workspace. Members manage their own agents and credentials; owner/admin may deactivate any workspace agent but cannot issue credentials for another member's agent. Proposed tokens use 32 cryptographically random bytes with a recognizable version prefix; store SHA-256 hashes and display plaintext only on issuance. Tokens are high-entropy secrets, so password stretching is unnecessary.

Rotation atomically revokes the selected credential and issues its replacement. Removing a member or deactivating an agent makes all associated credentials unusable. Every MCP request checks the credential, agent activation, and current owner membership directly in PostgreSQL, with no authorization cache. Tool mutations recheck authorization transactionally so a request cannot reuse stale authorization after a committed revocation. Membership/agent/credential row locks establish the transaction ordering: work already committed before revocation remains committed; subsequent work is denied.

Browser session cookies never authenticate MCP. The authenticated context supplies the agent and workspace; tools expose no caller-controlled sender or workspace fields. Human admins have no mailbox-content privilege.

### 6. Official remote MCP and privacy boundary (change 4)

Wrap the SDK `StreamableHTTPHandler` at `/mcp` with body limits, explicit Origin checks, rate limits, and agent bearer authentication. Use `Stateless: true` and `JSONResponse: true` for polling; GET/DELETE behavior follows the SDK (405 in stateless mode). No custom transport or JSON-RPC implementation is added. Two clients still create separate SDK client connections, even though the server does not retain sessions.

SDK v1.8.0 does not enable cross-origin protection by default. Validate any supplied Origin against the exact configured allowlist on every MCP method; reject `null`, malformed, or untrusted origins with 403. Permit absent Origin for authenticated native clients. Reject wildcard credentialed CORS. Host validation must permit Railway's healthcheck host only for health routes. Origin validation complements authentication; it does not grant access.

These bearer credentials are an MVP integration mode, not full MCP OAuth discovery/authorization. Record actual tested Codex, Claude Code, and custom-client versions and header configuration in change 5; never claim compatibility from unexecuted examples. The executable Go SDK example is the baseline verified client.

### 7. Durable mailbox model and semantics (change 4)

Proposed tables: users, workspaces, memberships, invitations, browser sessions/OAuth attempts, agents, agent credentials, conversations, messages, per-inbox sequence counters, and audit events. IDs use UUIDs; GitHub IDs are bigint. Tenant-bearing relationships use composite workspace foreign keys. Mailbox queries require authenticated workspace plus recipient ID; no admin endpoint returns message content.

The public message fields are `id`, `workspace_id`, `from_agent_id`, `to_agent_id`, `thread_id`, `in_reply_to`, `kind`, `text`, `metadata`, `created_at`, and `acknowledged_at`. Proposed tools:

| Tool | Caller input | Result |
| --- | --- | --- |
| `whoami` | none | authenticated agent and workspace |
| `list_agents` | limit, cursor | active agents in this workspace |
| `send_message` | recipient ID, idempotency key, kind, text, metadata, optional thread/reply | committed message |
| `read_messages` | limit, cursor, acknowledged status, optional thread/kind | own-inbox page |
| `acknowledge_message` | message ID | own message acknowledgement |

Defaults: page size 50, maximum 100; text at most 16 KiB UTF-8; metadata JSON object at most 8 KiB encoded and depth at most 8; kind at most 64 ASCII characters; idempotency key 1–128 ASCII characters; serialized result at most 256 KiB. Bound pagination by bytes as well as count, with continuation when a page hits either bound. Return stable structured application error codes through SDK tool results: `invalid_argument`, `not_found`, `forbidden`, `idempotency_conflict`, `rate_limited`, `temporarily_unavailable`. Authentication failures use HTTP 401; origin failures 403; body overflow 413; throttling 429. Malformed MCP requests retain SDK protocol errors.

Successful send means a PostgreSQL commit. Idempotency uniqueness is `(workspace_id, from_agent_id, idempotency_key)` across credential rotations. Hash a canonical representation of validated caller payload, including recipient and optional reply/thread references; persist that fingerprint and the resolved result. Repeating an identical payload returns its existing message; a changed payload conflicts. Lookup the existing idempotency result before generating defaults such as a new thread ID.

A new conversation records its two participants; a reply must reference a message visible to the caller and preserve that participant pair and workspace. Reject invisible references without confirming their existence. Validate recipient activation and membership for new deliveries. Read and acknowledge select only the authenticated inbox. Reading does not acknowledge; acknowledging twice returns the original acknowledgement timestamp.

Use a transactionally incremented per-inbox sequence, locking that inbox counter until send commits. This ensures a lower sequence cannot commit after a higher sequence and disappear behind a paging cursor. Read pages ascend by sequence, with an opaque cursor bound to agent, workspace, filters, and snapshot upper sequence. Messages committed beyond the snapshot appear on the next fresh poll. Acknowledgement filters are evaluated when each page is read; acknowledgements may remove entries from an unacknowledged scan. This is a delivery snapshot, not a historical snapshot of acknowledgement state.

Multiple clients sharing an agent identity can read the same messages and race to acknowledge. There is no claim/lease or single-consumer guarantee. Unacknowledged messages remain available on a fresh read, and recipients must tolerate repeated processing. Do not promise exactly-once execution.

Proposed retention policy: retain messages and idempotency records indefinitely in MVP, with no automatic deletion. Capacity monitoring and an explicitly approved future retention change are required before pruning. Message text and metadata are untrusted external data, never system instructions.

### 8. Limits, audit, and operations

Start with one Railway application replica. Proposed MCP limits are 60 authenticated requests per minute per agent, burst 20; login attempts 20 per minute per IP, burst 5. Bounded in-process rate-limit state is sufficient for one replica; document that limits are per replica. A distributed rate limit must be specified before scale-out rather than claiming a global guarantee. Query limits and database constraints protect storage independently.

Audit role changes, invitation lifecycle, agent lifecycle, credential issuance/revocation/rotation, and bootstrap. Store actor, workspace, action, target ID, outcome, and timestamp, without secret values or message content. Domain changes and their audit records commit in one transaction. Use PostgreSQL for durable sessions and audit data; do not write durable state to Railway application storage.

### 9. Ordered delivery and final verification

| Change | Scope | Required proof before archive |
| --- | --- | --- |
| 1 `project-foundation` | runtime, migrations, containers, CI, documentation | all foundation requirements and scenarios |
| 2 `identity-workspaces` | OAuth, sessions, roles, invites, bootstrap | mocked OAuth plus real-DB invitation/role/concurrency tests |
| 3 `agent-credentials` | agent ownership, tokens, rotation/revocation | own-agent policy and immediate access denial tests |
| 4 `mcp-mailbox` | five SDK tools, durable delivery, executable example | two separate HTTP MCP connections, different members, send/read/ack/reply/read |
| 5 `web-management-readiness` | management pages, CSRF, full integration, runbooks | complete fresh-machine reproduction, isolation, restart persistence, documented client checks |

Each change creates proposal, delta specs, design, and tasks before its implementation; update specs when behavior changes. Archive only after requirement verification, with observed results recorded. The final gate covers different-workspace isolation, sender spoofing, foreign inbox/reply denial, invalid invitations, last-owner protection, revocation/removal, concurrency, idempotency, pagination, and persistence across application restart.

The request/reply example launches two independent clients with separate credentials and bounded polling loops. An idle or stopped Codex/Claude process does not wake because a message arrives. Runners, webhooks, and push activation require a future change.

## Risks / Trade-offs

- [Indefinite storage] → monitor database growth; propose retention before introducing deletion.
- [Bearer secret leakage] → one-time display, hashed storage, prompt revocation, redacted logs; no OAuth interoperability claim.
- [Strict schema compatibility] → rehearse upgrades; do not promise zero downtime; define future compatibility windows explicitly.
- [Per-inbox serialization] → predictable cursor ordering at the cost of throughput to one recipient; benchmark before adding infrastructure.
- [One replica rate limiting] → expose the limitation in operations documentation and require a scale-out design change.
- [External service setup] → GitHub OAuth and Railway require owner-supplied configuration; local mocks cannot prove live deployment.

## Migration Plan

After approval and repository-name confirmation, implement foundation, verify locally, and capture evidence. Prepare a multi-stage digest-pinned image and Railway configuration with `mailbox migrate` pre-deploy and `/readyz` deployment check. Ensure the migration binary exists in the runtime image; Railway pre-deploy filesystem changes are not persistent. Set the application migration deadline to five minutes and configure a compatible Railway timeout.

Document `DATABASE_URL`, `$PORT`, production TLS/domain, GitHub callback at `/auth/github/callback`, secret setup, database backup ownership, and tested restore steps as their features become available. Railway healthchecks gate deployment but do not provide continuous uptime monitoring. Before any later authorized deployment, verify database backups and restore on an isolated database. No cloud resources, GitHub repository, or public deployment are created during this planning change.

## Approval and Deferred Inputs

The user approved the first specification and architecture and confirmed `petarnenov/bot-space`; see `approval.md`. The project name is `bot-space` and module path is `github.com/petarnenov/bot-space`. Production domain, OAuth app identifiers, Railway project, and backup schedule can be supplied during deployment preparation without changing the foundation behavior.

## Official Sources and Version Checks

Checked on 2026-10-08:

- [OpenSpec installation](https://openspec.dev/docs/installation), [CLI](https://openspec.dev/docs/cli); npm registry reports 1.14.1 and the installed CLI confirms it.
- [Go stable releases](https://go.dev/dl/?mode=json): 1.27.2, with 1.26.9 also listed stable.
- [Official MCP Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0), [versioned go.mod](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/go.mod), [Streamable HTTP implementation](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/streamable.go).
- [pgx v5.11.0](https://github.com/jackc/pgx/releases/tag/v5.11.0).
- [PostgreSQL versioning](https://www.postgresql.org/support/versioning/): supported PostgreSQL 18, current minor 18.6.
- [GitHub OAuth web flow](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps): state, S256 PKCE, callback handling, and user identity lookup.
- [MCP transport security](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports): Origin validation and HTTP transport semantics.
- [Railway Dockerfiles](https://docs.railway.com/builds/dockerfiles), [healthchecks](https://docs.railway.com/deployments/healthchecks), [pre-deploy commands](https://docs.railway.com/deployments/pre-deploy-command).

Container manifests, vulnerability-scanner version, GitHub Actions SHAs, and any additional dependencies must be checked and locked during implementation; they are not fabricated here.

Implementation dependency correction: vulnerability scanning identified GO-2026-6629 and GO-2026-5970 in pgx’s transitive `golang.org/x/text` v0.29.0; explicitly pin v0.41.0, which contains both fixes.
