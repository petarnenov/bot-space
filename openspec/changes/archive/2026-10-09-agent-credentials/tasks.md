# Tasks

The user approved the architecture and authorized autonomous sequential implementation. All tasks are verified complete; see `verification.md`. Agent credential specifications precede implementation; MCP remains a later change.

## 1. Agent Persistence and Ownership

- [x] 1.1 Add schema-qualified agents/credentials migration with tenant-consistent relationships and membership-delete access revocation/audit; verify real-PostgreSQL migration/repeat/integrity tests and removal/rejoin isolation tests.
- [x] 1.2 Implement protected browser POST routes and member-owned registration, valid display names, own-agent listing, and authorized/idempotent deactivation; verify different-member IDs/ownership, invalid/foreign registration, own listing, member substitution denial, admin same-team deactivation and foreign-admin denial.
- [x] 1.3 Record agent lifecycle audits atomically and document activation/ownership semantics; verify audit count/metadata and rollback when audit insertion fails.

## 2. Credential Lifecycle

- [x] 2.1 Implement protected browser issuance/rotation/revocation routes and random versioned one-time bearer issuance, hash-only persistence, metadata-only listing, and strict owner/current-membership authorization; verify separate agent tokens, stored hashes, no redisplayed secrets, inactive-agent denial, and administrator inability to issue another member's credentials.
- [x] 2.2 Implement idempotent revoke and atomic selected-token rotation with safe audit records; verify old/new/unaffected token status, repeat revoke, refused inactive/revoked rotation, and transactional rollback on audit failure.

## 3. Request Authentication and Removal

- [x] 3.1 Implement bounded uncached bearer HTTP authentication with private principal context and sanitized denial; verify real HTTP requests for valid token identity, spoofed parameters, cookie-only, missing/malformed/duplicate/unknown/revoked credentials and database failure.
- [x] 3.2 Implement transaction-bound authorization recheck with membership/agent/credential lock order; verify concurrent held authorization against revocation/deactivation/removal, then stale-context denial after commit.
- [x] 3.3 Set verified audit actor for membership deletion and verify trigger revokes/deactivates only the removed workspace, retains records, emits no secrets, and never revives credentials after a new invitation.

## 4. Integrated Verification

- [x] 4.1 Document credential handling, bearer-versus-OAuth limits, current client verification status, and member-removal semantics; run formatting, module checks, vet, tests/race with PostgreSQL, build, vulnerability scan, and strict OpenSpec validation, recording every requirement/scenario in `verification.md`.
- [x] 4.2 Build and smoke-test the additive migration/runtime container with previous identity/foundation behavior preserved and MCP still unavailable; record results and keep deployment unpublished.

## Requirement-to-Check Mapping

| Requirement | Tasks |
| --- | --- |
| Scoped agent registration; Own-agent management; Agent deactivation | 1.2 |
| Agent lifecycle audit | 1.3 |
| One-time high-entropy credentials; Credential ownership | 2.1 |
| Atomic revocation and rotation; Credential audit and interoperability limits | 2.2, 4.1 |
| Uncached agent authentication | 3.1 |
| Transaction-bound authorization recheck | 3.2 |
| Removed member agent access termination | 1.1, 3.3 |

## Workflow follow-up

- Sync and archive only after all tasks and scenarios are verified.
- Create `mcp-mailbox` proposal/specs/design/tasks before implementing the official SDK transport and durable message delivery.
