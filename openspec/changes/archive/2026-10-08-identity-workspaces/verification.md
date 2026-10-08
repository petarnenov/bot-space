# Identity and Workspace Verification

Executed locally on 2026-10-08 with Go 1.27.2 and real PostgreSQL 18.6. The user approved the first architecture and authorized sequential implementation. This change adds no agent credentials or MCP tools.

## Executed Checks

- Go build, vet, module verification, formatting, and strict OpenSpec validation: pass.
- Existing foundation integration tests plus new identity/workspace tests against isolated real PostgreSQL databases: pass.
- Full `go test -race -count=1 ./...` with `TEST_DATABASE_URL`: pass. Later additional security assertions were verified with targeted race runs for invitation secrets/isolation, audit rollback, and missing/browser-unbound OAuth state/token responses.
- Vulnerability scanning with govulncheck v1.8.0: no vulnerabilities found.
- Actual compiled `bootstrap-owner` CLI, run twice against a disposable PostgreSQL database: one workspace/owner and one system audit event. A different GitHub ID with the same slug exits nonzero with conflict.
- Multi-stage container build: pass. The new image applies both migrations, starts healthy/ready, and serves configured login/root routes with a fixed GitHub authorization URL. `/mcp` remains 404. No external OAuth request was sent during that smoke check.
- Foundation lifecycle smoke still passes: application replacement, ordinary Compose down/up, database outage/recovery, and persisted fixture.

A timeout mock initially left its server handler unbounded. The fixture was corrected to consume the request body and bound its own exit; the provider timeout scenario then completed with sanitized denial after approximately ten seconds. The abandoned inactive synthetic test database was removed.

## Requirements and Scenarios

| Requirement | Authoritative evidence |
| --- | --- |
| GitHub identity | `TestOAuthHTTPIdentityAndSessionRotation` exercises redirect/cookie/provider HTTP flow, immutable GitHub ID across username change, no login-created team membership, and hashed session storage with no provider token persistence. |
| OAuth state and PKCE | The same HTTP flow asserts S256 challenge/verifier and exact callback. `TestOAuthStateMismatchExpiryReplayAndSafeRedirect` rejects mismatched, missing, browser-unbound, expired, and replayed state before exchange. |
| Bounded OAuth provider calls | `TestOAuthProviderFailuresAreSanitized` covers provider HTTP failure, missing token, oversized body, nonpositive ID, and the actual ten-second timeout. No failed flow creates a session or reflects provider secrets. Production provider disables redirects and has fixed GitHub URLs. |
| Server-side browser sessions | Login rotation rejects the old cookie while keeping the immutable user ID. `TestSessionExpiryCSRFOriginLogoutAndCookieFlags` covers idle/absolute expiry; `TestSessionCookieProtections` verifies Secure/HttpOnly/Lax/host-only/path/lifetime settings and expiry cookies. |
| Logout and browser mutation protection | Session test verifies wrong CSRF, foreign origin, absent origin/referrer denial, valid POST logout, and revoked-cookie rejection. `TestInvitationHTTPRequiresSessionCSRFAndTarget` proves the same protection on actual POST acceptance and denies GET mutation. |
| Configuration and safe redirects | `TestIdentityConfig` validates all-absent/partial settings, HTTPS/loopback, credentials/path/query/port errors. `TestSafeReturn` rejects external, protocol-relative, encoded slash/backslash/control, secret-query and fragment targets. HTTP callback confirms unsafe return falls back to `/`. |
| Isolated multiple memberships | `TestWorkspaceIsolationBootstrapAndRoles` proves multiple teams via invitation, foreign member denial, and workspace-filtered listing. HTTP home shows own team only. |
| Administrative bootstrap | Actual binary checks plus service test prove first/repeated/conflicting bootstrap and system audit actor. User-facing operator commands are in `docs/identity.md`. |
| Role administration | Role test denies member administration, admin self-escalation, owner removal/demotion, and verifies successful authorized role change/removal. Caller membership and role are checked transactionally. |
| Last owner invariant | Single-owner rejection plus `TestLastOwnerConcurrency`: one of two concurrent demotions succeeds, one is denied, and one owner remains. |
| Transactional role auditing | Role audit count/metadata assertions and `TestMembershipMutationRollsBackWhenAuditFails` demonstrate state and audit commit together or both roll back. |
| Targeted invitation issuance | `TestInvitationsIdentityExpiryCancellationAndAudit` verifies 48-hour expiry, SHA-256-only storage, valid issuance, invalid role/ID/member actor/current-member rejection, and secret-free audit. |
| One-time identity-bound acceptance | That test rejects wrong account, wrong/equal-length wrong token, expiry, reuse and cancellation. `TestInvitationConcurrentAcceptance` commits one grant. HTTP acceptance derives identity from the session despite a spoofed user-ID form field. |
| Invitation cancellation and isolation | Cancellation is idempotent; member/foreign listing and other-workspace cancellation are denied. Returned invitation structs contain no secret/hash fields. |
| Invitation audit metadata | Lifecycle test verifies exactly the committed create/accept/cancel events, no duplicate cancellation event, and no plaintext invitation secret. |
| Modified private-feature availability | Configured-container smoke exposes identity routes and health; operations-only foundation routes retain MCP/management 404. HTTP authenticated home shows only the caller's workspace memberships. |

Browser pages use escaped `html/template`, no-store, no-referrer, CSP, and nosniff; `TestIdentityHTMLIsEscaped` and HTTP header assertions verify those protections. Domain SQL is parameterized and schema-qualified. Sessions, invitations, role state, and audit reside in PostgreSQL.

## External Limits

GitHub OAuth is verified through a local mock over HTTP, not a live registered OAuth app. Hosted GitHub Actions, public repository publication, paid provisioning, and Railway deployment remain unexecuted. The configured-container authorization redirect was inspected without following it to GitHub. Complete management UI belongs to change 5; agent access revocation on member removal is implemented in change 3 before the product is considered ready.
