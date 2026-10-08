# MCP Mailbox Tools

## Purpose

Expose a provider-neutral private agent mailbox through the official Streamable HTTP transport and bounded, documented MCP tools.

## ADDED Requirements

### Requirement: Official authenticated remote transport
When mailbox configuration is enabled, `/mcp` SHALL use the official Go SDK Streamable HTTP handler with stateless JSON responses. Every request SHALL authenticate a current agent bearer credential before SDK dispatch; cookies alone SHALL NOT authenticate. The server SHALL NOT implement its own MCP or JSON-RPC transport or launch client agents.

#### Scenario: Two independent clients
- **GIVEN** agents owned by different users in one workspace with distinct credentials
- **WHEN** two separate SDK clients connect over HTTP
- **THEN** each discovers and invokes mailbox tools under its own credential-derived identity.

#### Scenario: Unauthorized protocol request
- **GIVEN** missing, invalid, revoked, or cookie-only credentials
- **WHEN** a request reaches `/mcp`
- **THEN** SDK dispatch is denied with HTTP 401, or sanitized unavailability when database verification fails.

### Requirement: MCP Origin and body protection
Every MCP HTTP method SHALL reject a present malformed, null, duplicate, or untrusted Origin with HTTP 403. An absent Origin SHALL be allowed for authenticated native clients. Bodies above 1 MiB SHALL receive 413, including chunked input. SDK localhost/DNS-rebinding protection SHALL remain enabled unless an equivalent configured host policy is verified.

#### Scenario: Untrusted browser origin
- **GIVEN** a valid token and a foreign/null/malformed/duplicate Origin
- **WHEN** any MCP method is requested
- **THEN** it returns 403 without tool execution.

#### Scenario: Native client and oversized body
- **GIVEN** a native request without Origin or an oversized request body
- **WHEN** it reaches the MCP boundary
- **THEN** the valid native request can proceed while oversized input receives 413.

### Requirement: Credential-derived identity and recipient directory
`whoami` SHALL accept only an empty object and return agent ID, name, owner ID and workspace ID from current credentials. `list_agents` SHALL return only active eligible agents whose owners remain members of that workspace, including offline recipients, with bounded pagination. Neither tool SHALL expose credentials, inbox content, or a caller-selected identity.

#### Scenario: Isolated directory and identity
- **GIVEN** several users' agents in one workspace and agents in another
- **WHEN** whoami and list_agents are invoked
- **THEN** identity matches the token and the directory excludes the other workspace and inactive agents.

### Requirement: Explicit mailbox tool schemas
The five tools SHALL publish object input schemas with no unknown properties. `send_message` SHALL require recipient UUID, client idempotency key and text, with optional kind/metadata/thread/reply. `read_messages` SHALL accept own-inbox pagination plus acknowledgement/thread/kind filters. `acknowledge_message` SHALL accept only a message UUID. Sender/workspace/inbox identity fields SHALL NOT be accepted as arguments.

#### Scenario: Spoofed tool identity
- **GIVEN** valid credentials and sender/workspace/foreign-inbox arguments
- **WHEN** a mailbox tool is called
- **THEN** it returns `invalid_argument` and cannot replace its authenticated identity.

### Requirement: Tool limits
Text SHALL be 1–16384 UTF-8 bytes without NUL; encoded object metadata at most 8192 bytes/depth 8. Kind SHALL be 1–64 ASCII letters/digits or `_.:-`, defaulting to `message`. Keys SHALL be 1–128 printable ASCII characters. Pages SHALL default to 50 and cap at 100. Serialized MCP tool results SHALL be at most 256 KiB.

#### Scenario: Invalid payload or page limit
- **GIVEN** exceeded payload/depth limits, malformed UUIDs, unknown fields, or page limit outside 1–100
- **WHEN** the tool is called
- **THEN** it returns bounded `invalid_argument` without persisting changes or echoing private input.

#### Scenario: Large inbox page
- **GIVEN** many maximum-sized messages
- **WHEN** an inbox page is requested
- **THEN** count and serialized-result byte limits are both respected, with continuation for remaining results.

### Requirement: Stable application failures
Application failures SHALL return bounded tool error data with stable codes: `invalid_argument`, `not_found`, `forbidden`, `idempotency_conflict`, `rate_limited`, and `temporarily_unavailable`. Private inputs and raw SQL errors SHALL NOT be echoed. SDK protocol errors SHALL remain SDK-defined.

#### Scenario: Application or protocol error
- **GIVEN** invalid application data or a malformed protocol request
- **WHEN** MCP processes it
- **THEN** application failures use the documented code envelope, while protocol failures retain SDK semantics, without private input disclosure.

### Requirement: Executable two-client request reply
The repository SHALL include a runnable example with separate requester/responder processes, separate credentials, and bounded inbox polling through the official SDK. Documentation SHALL explain that inactive Codex/Claude processes are not awakened by messages and that runners, webhooks and push activation are future work.

#### Scenario: Request reply demo
- **GIVEN** two eligible agents and their independent configured client processes
- **WHEN** requester sends and both poll
- **THEN** responder reads, acknowledges and replies, and requester reads the reply over HTTP MCP.
