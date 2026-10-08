# Tasks

The first architecture was approved and the user authorized autonomous execution of the agreed sequential changes. All tasks below are verified complete; see `verification.md`. Identity/workspace specifications precede implementation. This change does not claim agent or mailbox readiness.

## 1. Identity Persistence and Configuration

- [x] 1.1 Add an additive, schema-qualified identity/workspace migration with UUID users, immutable GitHub IDs, workspaces, membership roles, invitations, OAuth attempts, sessions, and safe audit metadata; verify bundled migrations and real-PostgreSQL migration/repeat/integrity tests.
- [x] 1.2 Add shared crypto-random identifiers/secrets, hashed credential identifiers, and validated identity environment configuration; verify invalid/partial configuration, production HTTPS, loopback development, and unsafe return-path tests.
- [x] 1.3 Extend bounded HTTP route registration without bypassing foundation middleware and document configuration/operations-only behavior; verify disabled identity routes and `/mcp` remain unavailable while health still works.

## 2. GitHub Login and Browser Sessions

- [x] 2.1 Implement state/PKCE OAuth attempts, fixed production endpoints, bounded provider HTTP calls, immutable-ID upsert, and session rotation; verify a mocked provider over HTTP, matching verifier/callback, username change, no automatic membership, state mismatch/expiry/replay, provider failure, and token nonpersistence.
- [x] 2.2 Implement hashed server-side sessions with absolute/idle expiry, cookie protections, same-origin CSRF checks, and POST logout; verify fixation prevention, expiry, cookie flags, rejected foreign/missing-CSRF mutations, and revoked-cookie replay.
- [x] 2.3 Add minimal escaped/no-store login/logout and workspace-selection HTML, with no inbox access, and document the exact callback/setup; verify authenticated page isolation and browser response protections over HTTP.

## 3. Workspace Administration

- [x] 3.1 Implement isolated multiple-workspace/member listing, transactionally authorized role/removal policy, and serialized last-owner protection with safe audit events; verify cross-workspace denial, member/admin restrictions, successful role audit, and concurrent owner changes against PostgreSQL.
- [x] 3.2 Implement administrative `bootstrap-owner` with safe argument parsing and idempotent/conflict behavior; execute the binary twice against PostgreSQL, verify one owner/workspace, refusal for another target, and a system audit actor. Document exact operator commands.

## 4. Targeted Invitations

- [x] 4.1 Implement owner/admin issuance of hashed one-time invitation secrets, fixed 48-hour expiry, member/admin role constraints, and secret-free listing; verify issuance, invalid IDs/roles/actors/current-member conflicts and transactional audit.
- [x] 4.2 Implement workspace-qualified cancellation and target-ID-bound atomic acceptance; verify wrong account/token, expiry, reuse, cancellation idempotency, foreign workspace denial, concurrent acceptance, and lifecycle audit without secrets. Document invitation semantics and POST-only acceptance.

## 5. Integrated Verification

- [x] 5.1 Run formatting, module verification, vet, full tests with real PostgreSQL, race detector, build, vulnerability scan, and strict OpenSpec validation; record commands/results and requirement/scenario traceability in `verification.md`, distinguishing mock/local results from live GitHub/hosted CI/Railway.
- [x] 5.2 Verify the built container can apply the additive migration and serve configured identity routes with health/readiness, while existing foundation lifecycle checks continue to pass; record evidence and keep deployment unpublished.

## Requirement-to-Check Mapping

| Requirement | Tasks |
| --- | --- |
| GitHub identity; OAuth state and PKCE; Bounded OAuth provider calls | 1.1, 2.1 |
| Server-side browser sessions; Logout and browser mutation protection | 1.1, 2.2 |
| Configuration and safe redirects | 1.2, 1.3, 2.1 |
| Isolated multiple memberships | 2.3, 3.1 |
| Administrative bootstrap | 3.2 |
| Role administration; Last owner invariant; Transactional role auditing | 3.1 |
| Targeted invitation issuance | 4.1 |
| One-time identity-bound acceptance; Invitation cancellation and isolation; Invitation audit metadata | 4.2 |
| Modified private-feature availability | 1.3, 2.3, 5.2 |

## Workflow follow-up

- Sync and archive only after every tracked task and scenario is verified.
- Create `agent-credentials` proposal/specs/design/tasks before its implementation; extend member removal to revoke agent access transactionally.
