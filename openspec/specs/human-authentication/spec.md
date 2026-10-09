# human-authentication Specification

## Purpose

Identify human users through GitHub without repository permissions and maintain protected, revocable browser sessions.

## Requirements

### Requirement: GitHub identity
The service SHALL authenticate humans only through GitHub OAuth, key their identity by immutable positive GitHub numeric user ID, and update display usernames without changing identity. It SHALL request no repository or email scope and SHALL NOT persist GitHub access or refresh tokens after identity retrieval. Login alone SHALL grant no workspace membership.

#### Scenario: Username changes
- **GIVEN** an existing user whose GitHub username changed
- **WHEN** login returns the same numeric GitHub ID with the new username
- **THEN** the same local user and memberships remain, with the new display name.

#### Scenario: New login has no team access
- **GIVEN** a successful login by a new GitHub account
- **WHEN** its session lists workspaces
- **THEN** the result is empty until invitation acceptance or administrative bootstrap.

### Requirement: OAuth state and PKCE
Every OAuth attempt SHALL use random state bound to the initiating browser, a ten-minute expiry, and S256 PKCE. Callback SHALL reject missing, mismatched, expired, or previously consumed state before exchanging a code. State SHALL be consumed atomically. The token exchange SHALL send the original verifier and exact configured callback URL.

#### Scenario: Successful protected flow
- **GIVEN** an unexpired attempt in its initiating browser
- **WHEN** the callback supplies matching state and a valid code
- **THEN** one exchange uses the original PKCE verifier and callback, followed by authenticated user retrieval.

#### Scenario: Invalid or replayed state
- **GIVEN** mismatched, expired, missing, or consumed state
- **WHEN** callback processing occurs
- **THEN** no token exchange or authenticated session creation occurs.

### Requirement: Bounded OAuth provider calls
OAuth token exchange and identity retrieval SHALL use bounded response bodies and a ten-second total flow deadline, verify HTTP success and valid response fields, and return sanitized failures. Production endpoints SHALL be fixed to GitHub; test injection SHALL NOT be configurable through production environment variables.

#### Scenario: Provider error or invalid identity
- **GIVEN** a provider timeout, failed HTTP response, missing token, or nonpositive user ID
- **WHEN** login callback runs
- **THEN** no browser session is established and no provider payload or token appears in errors or logs.

### Requirement: Server-side browser sessions
Sessions SHALL use cryptographically random opaque cookie values, hashed identifiers stored in PostgreSQL, eight-hour absolute expiry, and 30-minute idle expiry. Login SHALL invalidate any prior session and issue a fresh ID. Production cookies SHALL be Secure, HttpOnly, SameSite=Lax, and path-scoped to `/`. Browser sessions SHALL NOT authenticate MCP.

#### Scenario: Session fixation prevention
- **GIVEN** a browser carrying an existing session before login
- **WHEN** login succeeds
- **THEN** a new cookie is issued and the prior session can no longer authenticate.

#### Scenario: Expired session
- **GIVEN** a session beyond its idle or absolute deadline
- **WHEN** a protected request is made
- **THEN** it is rejected without workspace access.

### Requirement: Logout and browser mutation protection
Logout SHALL require POST, an authenticated session, and a session-bound CSRF token; it SHALL revoke the session and expire the cookie. All browser mutations SHALL use this protection. Explicit `Origin` or `Referer` evidence SHALL match the public origin. When both are absent, the valid CSRF token SHALL provide mutation proof, whether Fetch Metadata is same-origin or unavailable. GET SHALL NOT perform logout or role/membership changes.

#### Scenario: Logout revokes a cookie
- **GIVEN** a valid session and CSRF token
- **WHEN** same-origin POST logout succeeds
- **THEN** replaying its old cookie cannot access protected pages.

#### Scenario: Forged mutation
- **GIVEN** missing or incorrect CSRF data
- **WHEN** logout or another browser mutation is attempted
- **THEN** it is rejected with HTTP 403 and state remains unchanged.

#### Scenario: Referrer-suppressed same-origin form
- **GIVEN** a valid session-bound CSRF token and a browser request with no `Origin` or `Referer`
- **WHEN** Fetch Metadata is same-origin or unavailable
- **THEN** the mutation is evaluated normally instead of being rejected for missing browser metadata.

#### Scenario: Foreign origin remains authoritative
- **GIVEN** a mutation carrying an explicit foreign `Origin` or `Referer`
- **WHEN** Fetch Metadata is absent, contradictory, or claims same-origin
- **THEN** the mutation is rejected without changing state.

### Requirement: Configuration and safe redirects
Complete `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and `PUBLIC_BASE_URL` SHALL enable identity routes. Partial/invalid configuration SHALL fail startup without exposing values. HTTPS SHALL be required except for loopback development. Return paths SHALL be local paths without schemes, hosts, or protocol-relative redirects. With all identity settings absent, operations-only serving SHALL expose no login or membership routes.

#### Scenario: Unsafe return target
- **GIVEN** a requested return URL with an external host or protocol-relative form
- **WHEN** login begins
- **THEN** it is rejected or normalized to `/` and cannot cause an external post-login redirect.

#### Scenario: Disabled or incomplete configuration
- **GIVEN** no identity settings or only some settings
- **WHEN** serving starts
- **THEN** absent settings leave identity routes unavailable, while partial settings cause a sanitized nonzero startup failure.
