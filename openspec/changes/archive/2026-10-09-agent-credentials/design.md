# Design

## Context

Foundation and GitHub identity/workspaces are verified and archived. The user approved the complete MVP architecture and authorized sequential implementation. Workspace mutations already serialize under a workspace row lock, while browser identity is separate from future agent authentication.

## Goals / Non-Goals

**Goals:** scoped agent ownership, separate one-time bearer tokens, current-state authentication, and transactionally permanent access revocation.

**Non-Goals:** message storage, MCP SDK mounting/tools, agent runners, cross-workspace communication, full credential-management pages, or an OAuth authorization server.

## Decisions

### Persistence and credentials

Add `0003_agents_credentials.sql` with agents `(id,workspace_id,owner_user_id,name,active,created_at)` and credentials `(id,workspace_id,agent_id,secret_hash,created_at,revoked_at)`. Agent/workspace composite uniqueness and foreign keys prevent cross-workspace credential relationships. Preserve agent records on deactivation/removal so future messages retain historical senders/recipients.

Names are trimmed valid UTF-8, 1–128 bytes, with no controls. Names are display labels, not unique routing identifiers or vendor/model restrictions. Activation means permission to participate and does not imply a connected process.

Tokens use `bot_space_v1_` plus 32 random bytes encoded as unpadded base64url. Store SHA-256 of the entire opaque token, with a unique hash index. Return raw tokens only from issuance/rotation. Lists expose ID and lifecycle timestamps, never token/hash. No additional crypto dependency or password stretching is needed for high-entropy random secrets.

### Human ownership and lifecycle transactions

Human mutations lock the workspace, check actor membership, then lock the target agent. Members operate only on their own agents; owner/admin may deactivate another member's agent but cannot issue/rotate/list its credentials. Deactivation revokes all active credentials in the same transaction; repeat transitions produce no new effective events. Rotation changes only the selected active credential, and atomically issues its replacement. Inactive agents cannot issue/rotate tokens.

Safe audit records identify the human actor, workspace, agent/credential ID, action and timestamps. Rotation metadata may identify old/replacement credential IDs but never include hashes/raw tokens. State and audits commit together; failed audit insertion rolls back the operation.

### Permanent removal revocation

A database membership-delete trigger deactivates that member's scoped agents, revokes all scoped credentials, and writes safe lifecycle events before membership deletion commits. This guarantees revocation regardless of which application call removes membership; an optional application callback could be accidentally omitted. The workspace service sets a transaction-local verified audit actor before deletion. Direct operator SQL without that setting audits a system actor. The trigger preserves agent records and touches no other workspace.

The lock order is workspace mutation lock, owner membership, agent, credential. The trigger holds the removed membership's exclusive lock before agent/credential changes. Rejoining creates membership again but cannot revive inactive agents or revoked credentials; the user registers new agents for new access.

### Authentication and stale-context defense

A bounded HTTP middleware accepts exactly one Authorization bearer field, validates the token form, and queries PostgreSQL every request. It ignores browser cookies and any caller-supplied identity fields. Missing/malformed/duplicate/revoked/inactive/no-membership requests never reach protected handlers. Database failures deny access with a sanitized response. No JWT, local authorization cache, or full MCP OAuth discovery is introduced.

Authentication first finds candidate identities by hash, then rechecks and takes shared locks on owner membership, agent and credential in that order. Revocation/deactivation/removal take corresponding exclusive locks. A transaction-bound `AuthenticateTx` supports later mailbox tools so middleware authentication cannot be reused after access was revoked. Requests completed before revocation may remain committed; no request started after committed revocation may authenticate the old credential.

The middleware transaction completes before calling HTTP handlers. Mailbox tools must perform another recheck in their own transaction; this avoids holding database connections through the entire SDK transport. Tests mount a protected HTTP probe solely to verify the boundary. The production `/mcp` remains unavailable until change 4, so a successful probe does not claim a working MCP server.

### Verification and documentation

Use real PostgreSQL with agents owned by different GitHub users in a shared team and a second isolated team. Verify ownership, role exceptions, hashes/one-time presentation, separate credentials, rotation/revoke semantics, all-token deactivation, permanent member removal and rejoining, safe audits, and transactional rollback on audit failure.

HTTP probe tests verify missing/invalid/duplicate/cookie-only denial and credential-derived principal despite spoofed parameters. Concurrency tests hold transaction authorization locks while revocation/removal waits, then verify denial after commit. Continue all earlier identity/foundation tests and race checks. Document token mode and current untested MCP-client status honestly; SDK/client integration is change 4/5.

## Risks / Trade-offs

- [Long-lived bearer secrets] → one-time display, hashed storage, rotation/revocation, TLS deployment, and no secret logging.
- [Authorization DB work] → correctness over cache throughput; tune within the bounded pool before optimizing.
- [Membership-delete trigger] → explicit versioned SQL, tenant-qualified updates, deterministic row order, and integration coverage; use a verified transaction-local actor only for audit attribution.
- [No reactivation] → deactivated historical agents remain for records; new registration issues a new identity.

## Migration Plan

Apply the additive migration using the existing serialized migrator. Old memberships remain intact; no messages or raw secrets are imported. After verification sync the specs and archive. MCP implementation must call transaction rechecks and must not infer full OAuth support from bearer authentication.

### Browser mutation delivery

Register protected POST endpoints for agent registration/deactivation and credential issuance/revoke/rotation under workspace/agent paths. Reuse the existing session, same-origin, and CSRF boundary. Actor identity always comes from the server-side session. Minimal server-rendered result pages expose agent/credential IDs and newly issued tokens only once, with no-store/no-referrer headers; full listing and management forms remain change 5. HTTP integration tests exercise ownership, spoofed actor fields, CSRF denial, and rotation/revoke through these actual routes.
