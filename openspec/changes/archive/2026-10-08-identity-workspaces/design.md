# Design

## Context

Foundation is verified and archived at `openspec/changes/archive/2026-10-08-project-foundation`. The user approved the MVP architecture and supplied `petarnenov/bot-space`. This change implements the next identity/team boundary without agent or message capabilities.

The existing runtime provides bounded bodies/timeouts, schema readiness, explicit migrations, and sanitized logging. Extend route registration while preserving this wrapper; do not create a second HTTP server or frontend infrastructure.

## Goals / Non-Goals

**Goals:** GitHub-only human identity, PostgreSQL-backed sessions, invitation-only isolated memberships, safe administration under concurrency, and an operator bootstrap command.

**Non-Goals:** agents, credentials, inboxes, MCP transport/tools, full management UI, live OAuth deployment verification, paid resources, or publishing.

## Decisions

### Persistence and identifiers

Add `0002_identity_workspaces.sql` in the existing schema with users (UUID plus unique positive GitHub bigint), workspaces (UUID/unique slug), composite memberships `(workspace_id,user_id)`, invitations, OAuth attempts, sessions, and audit events. All SQL names are schema-qualified. Generate high-entropy opaque secrets with `crypto/rand`; hash session identifiers, OAuth states, and invitation secrets with SHA-256. Store PKCE verifier only in a short-lived server-side attempt because exchange requires the original value. Do not store GitHub access/refresh tokens.

PostgreSQL constraints enforce role values and relation integrity. Use workspace-qualified queries for every team operation. Only global identity lookup uses GitHub ID independent of workspace.

### OAuth and browser context

Use GitHub's documented authorization-code flow with random browser-bound state, S256 PKCE, exact configured `/auth/github/callback`, explicitly empty scope, and identity retrieval from `/user`. Use bounded `net/http` requests with redirects disabled and a ten-second flow context; provider failures are sanitized. Fixed production authorize/token/user endpoints cannot be replaced by environment settings; tests inject a mock provider through an explicit Go constructor.

An HttpOnly SameSite=Lax attempt cookie binds the callback to the browser. Consume state atomically with an expiry condition before exchange. On success, upsert by GitHub numeric ID, revoke the prior browser session if present, issue a new 32-byte random ID, and redirect only to a validated local path. Reject scheme/host/protocol-relative/backslash/control-character returns. Allow HTTP public base URL only for loopback development; otherwise require HTTPS. When all identity settings are absent, retain documented operations-only mode. Any partial setting is a configuration error.

Server-side sessions use hashed IDs, an eight-hour absolute lifetime and 30-minute idle deadline. A session-bound random CSRF token is returned only through protected pages. Protected mutations require a valid session, constant-time CSRF comparison, and same-origin Origin/Referer validation. Native browser form submissions may omit Origin only when Referer verifies the configured origin; absence of both is rejected. OAuth's callback has its separate state/PKCE protection. Cookie attributes are Secure in production, HttpOnly, SameSite=Lax, Path=/; local loopback allows non-Secure development cookies. Successful logout revokes the database row and expires the cookie.

Identity responses use no-store, Referrer-Policy=no-referrer, HTML escaping, and bounded rendering. Initial pages provide login/logout and a user's workspace list; full management pages follow in change 5. No browser session authenticates MCP.

### Workspace mutation locking

Every workspace mutation locks its workspace row before reading actor/target roles or owner counts. This shared ordering serializes role changes and last-owner decisions. Admins cannot change owner roles or grant ownership; members cannot administer. Owners may grant ownership only while preserving the invariant. Removing a member removes its membership; the credentials change must additionally revoke/deactivate its agents transactionally before it can be considered complete.

Domain mutations and audit insertion occur in the same transaction. Audit records hold actor kind/ID, workspace, action, target ID and time; they never contain secret values. Operator bootstrap uses a system actor rather than impersonating a browser user.

### Bootstrap

`mailbox bootstrap-owner --github-user-id <id> --workspace <slug>` requires database configuration and access, with no public endpoint. Validate positive ID and a lowercase slug of 1–63 characters. Insert/find the immutable user and create the workspace plus owner membership in one transaction. Serialize concurrent bootstrap by the slug using a transaction advisory lock. Repetition is accepted only for a matching owner; a conflicting existing workspace is refused. Bootstrap does not require GitHub OAuth credentials or save a GitHub token.

### Invitations

Issue a random 32-byte secret and UUID invite ID, target GitHub ID, member/admin role, and 48-hour expiry. Return plaintext once; list metadata without secrets. Invite acceptance derives the actor from its session/service identity and compares the actual GitHub ID. Workspace then invitation row locking serializes acceptance and cancellation. Membership grant, consumed timestamp, and audit event are atomic. Reject wrong account, wrong token, expiry, cancelled/consumed invites, existing memberships, and unknown IDs with stable sanitized errors. Cancellation is idempotent; only committed lifecycle transitions receive success events.

Secret-bearing acceptance is a protected POST, not a GET mutation. The final invitation page may use a secret in a link but must suppress referrer/cache leakage and never persist plaintext in attempt/session return paths. No automatic cleanup of domain state is introduced. Expired OAuth attempt/session rows can be explicitly pruned later; expiration already prevents use.

### Verification and documentation

Use real PostgreSQL isolated databases. Exercise login against an `httptest` provider through browser redirects and cookies, assert PKCE/state/identity behavior and cookie/session lifecycle, and prove token values are absent from DB/application logs. Test expiry/replay/foreign origin and CSRF rejection. Exercise workspace/invitation services against PostgreSQL with multiple GitHub IDs, including last-owner concurrency, invitation replay, wrong identity, cancellation, expiry, and safe audit records. Keep existing foundation tests valid and extend bundled migration tests for the new migration.

Document exact OAuth callback, configuration, operations-only mode, bootstrap, roles, invitation lifecycle, and mocked-versus-live verification. Run formatting, vet, tests/race, build, vulnerability scanning, and strict OpenSpec validation before syncing/archiving.

## Risks / Trade-offs

- [Retained expired auth rows] → expiration checks deny access; housekeeping is deferred without a false cleanup claim.
- [Workspace serialization] → simple role/owner correctness at MVP scale; future optimization needs invariant-preserving tests.
- [Secret-bearing invitation links] → one-time hash storage, POST acceptance, no-referrer/no-store, and no secret-bearing return paths.
- [Operations-only configuration] → explicitly document missing identity features; complete OAuth settings enable actual login.
- [Live GitHub setup absent] → mock proves HTTP behavior, not production app registration or hosted callback.

## Migration Plan

Apply the additive identity migration with the existing command. Existing health-only operation remains available without OAuth settings. Configure the public base URL and GitHub OAuth app before enabling login. Bootstrap the initial workspace with operator CLI, then use targeted invitations. Production deployment remains subject to explicit permission.

## Official Sources

Checked on 2026-10-08: [GitHub OAuth authorization](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps), [scope definitions](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps), and [authenticated user API](https://docs.github.com/en/rest/users/users#get-the-authenticated-user). State and S256 PKCE are supported; identity is revalidated after every successful exchange. The foundation toolchain and dependency pins remain in force.

Implementation records: `/user` uses supported GitHub API version `2026-03-10`. Production cookies use the `__Host-` prefix with Secure/Path=/ and no Domain. Workspace records retain the original bootstrap GitHub ID so later role grants cannot turn another account into an idempotent bootstrap caller. Role audit metadata stores `role_from`/`role_to` atomically. Invitation expiry is rechecked against wall-clock time after row-lock acquisition.
