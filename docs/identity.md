# GitHub Identity and Workspaces

Accounts use immutable GitHub numeric user IDs. Usernames are display values.
Login alone creates no membership, and no public team signup endpoint exists.

## OAuth Setup

Create a GitHub OAuth app with callback
`https://YOUR_DOMAIN/auth/github/callback`. For loopback development use
`http://localhost:8080/auth/github/callback`. Configure all three environment
variables, or uncomment and fill them in local `.env` for Compose:

```text
GITHUB_CLIENT_ID=YOUR_OAUTH_CLIENT_ID
GITHUB_CLIENT_SECRET=YOUR_OAUTH_CLIENT_SECRET
PUBLIC_BASE_URL=https://YOUR_DOMAIN
```

The base URL is an origin without path, query, or credentials. HTTPS is required
outside loopback. Never commit real secrets. Production provider endpoints are
fixed to GitHub; integration tests inject a mock through Go constructors.

Absent identity settings retain operations-only health serving; partial settings
fail startup. `migrate` and `bootstrap-owner` require database configuration but
not GitHub credentials. OAuth requests no repository or email scope, uses
browser-bound single-use state and S256 PKCE, and discards GitHub tokens after
retrieving `/user`. The user API is pinned to supported version `2026-03-10`.

## Bootstrap and Invitations

Run migrations, then bootstrap with operator database access. Substitute the
intended account's immutable numeric ID, not its username:

```sh
go run ./cmd/mailbox migrate
go run ./cmd/mailbox bootstrap-owner --github-user-id 12345 --workspace my-team
# With the configured Compose environment:
docker compose run --rm migrate bootstrap-owner --github-user-id 12345 --workspace my-team
```

Matching bootstrap is idempotent; reusing the slug for another bootstrap account
is refused. There is no public bootstrap endpoint. Operator grants are audited
as system actions, without impersonating a browser user.

Owners/admins invite a nonmember GitHub ID into a member/admin role. A random
secret is displayed once, stored only as a SHA-256 hash, and expires after 48
hours. Cancellation is idempotent. Only the intended signed-in account can accept
with the valid secret; grant and consumption commit atomically. Forwarded,
expired, consumed, and cancelled invitations do not grant access.

The signed-in home page offers workspace selection and POST acceptance using
invitation ID/secret fields. Workspace pages provide invitation creation/listing/
cancellation and member role/removal forms. Invitation links carry their secret
through login in a short-lived HttpOnly cookie; accepting the invite clears it.
Browser mutations require session-bound CSRF and same-origin context. Secrets
must never enter OAuth return paths, logs, or redisplayed invitation lists.

Admins cannot remove/demote owners or grant ownership. Members cannot administer
others. Owners can grant ownership, and serialized role changes preserve at least
one owner. Domain mutations and safe audit metadata commit together.

## Sessions and Verification

Cookies contain random opaque IDs; PostgreSQL stores their hashes. Sessions have
eight-hour absolute and 30-minute idle expiry. Login rotates the ID and revokes
the previous session. POST logout revokes the row and expires the cookie.
Production uses Secure, HttpOnly, SameSite=Lax host cookies; loopback HTTP permits
non-Secure development cookies.

Mock GitHub HTTP tests against real PostgreSQL cover state/PKCE, identity,
sessions, provider failures, roles, invitation lifecycle, and concurrency. They
do not prove a live GitHub app or production callback. Live configuration and
deployment require owner settings and authorization. No hosted CI or Railway run
is claimed.

Sources: [OAuth web flow](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps),
[scopes](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps),
[API versions](https://docs.github.com/en/rest/about-the-rest-api/api-versions).
