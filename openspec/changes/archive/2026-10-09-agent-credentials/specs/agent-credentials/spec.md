# Agent Credentials

## Purpose

Authenticate each agent with separate revocable bearer credentials while deriving workspace and sender identity solely on the server.

## ADDED Requirements

### Requirement: One-time high-entropy credentials
Each agent SHALL support independent credentials containing at least 32 cryptographically random bytes. The service SHALL show a raw token only on issuance or rotation and persist only its cryptographic hash plus credential metadata. Credential listing SHALL never return raw tokens or hashes. Browser cookies SHALL NOT authenticate agent requests.

#### Scenario: Separate credentials
- **GIVEN** two registered agents
- **WHEN** credentials are issued
- **THEN** distinct raw tokens are returned once and database records contain only hashes.

#### Scenario: Credential listing and cookie-only request
- **GIVEN** an existing credential and authenticated browser session
- **WHEN** metadata is listed or an agent request uses only that cookie
- **THEN** listing contains no token/hash and the agent request receives HTTP 401.

### Requirement: Credential ownership
Only an agent's human owner with current workspace membership SHALL issue, rotate, revoke, or list its credentials. Owner/admin role SHALL NOT grant permission to issue credentials for another member's agent. Foreign agent/workspace IDs SHALL NOT bypass ownership checks.

#### Scenario: Administrator attempts another owner's issuance
- **GIVEN** an owner/admin managing another member's agent
- **WHEN** credential issuance or rotation is attempted
- **THEN** it is denied despite administrative role.

### Requirement: Uncached agent authentication
Every agent HTTP request SHALL require exactly one valid bearer credential and check its revocation status, agent activation, and current owner membership in PostgreSQL without an authorization cache. The authenticated agent, owner, and workspace SHALL derive from that credential; request parameters SHALL NOT replace that identity. Database failure SHALL deny access.

#### Scenario: Credential-derived identity
- **GIVEN** an active credential and owner membership
- **WHEN** authenticated requests carry spoofed sender/workspace parameters
- **THEN** the principal remains the credential's actual agent and workspace.

#### Scenario: Missing invalid or revoked credentials
- **GIVEN** missing, malformed, duplicate, unknown, or revoked credentials
- **WHEN** an agent request occurs
- **THEN** no protected handler runs and authentication is denied.

### Requirement: Atomic revocation and rotation
Revocation SHALL be idempotent and immediately deny subsequent authentication. Rotation SHALL commit revocation of the selected active credential and issuance of its replacement together. Inactive agents or revoked selected credentials SHALL NOT be rotated. Other active credentials of the same agent SHALL retain their own status.

#### Scenario: Rotation replaces one credential
- **GIVEN** an active agent with two credentials
- **WHEN** one is rotated
- **THEN** its old token fails, its new token succeeds, and the other credential remains valid.

#### Scenario: Repeated revocation
- **GIVEN** a credential
- **WHEN** revocation commits twice
- **THEN** it stays revoked with one effective revocation event.

### Requirement: Transaction-bound authorization recheck
Protected mailbox operations introduced later SHALL recheck credential, agent, and owner membership within their database transaction. Authorization locks SHALL order work against revocation/deactivation/removal so work committed before revocation remains committed, while subsequent operations cannot reuse a stale authenticated context.

#### Scenario: In-flight transaction and revocation
- **GIVEN** a transaction holding a validated agent principal and a concurrent revocation
- **WHEN** the transaction completes and revocation commits
- **THEN** later authentication and transaction rechecks deny the old token.

### Requirement: Credential audit and interoperability limits
Effective credential issuance, revocation, and rotation SHALL record safe audit events atomically with state changes, without raw tokens or hashes. Documentation SHALL describe the bearer mode as an integration credential mechanism, not a full MCP OAuth implementation, and distinguish tested clients from untested configurations.

#### Scenario: Safe credential lifecycle record
- **GIVEN** credential lifecycle operations
- **WHEN** audits and documentation are inspected
- **THEN** committed transitions have metadata events without secret material and no full-OAuth compatibility claim exists.
