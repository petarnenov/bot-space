# Proposal

## Why

The verified foundation has no human identity or team boundary. The next change establishes GitHub login and invitation-only membership so agent credentials can later inherit a reliable owner and workspace.

## What Changes

- Add GitHub OAuth with immutable user IDs, single-use state, S256 PKCE, fixed callbacks, and server-side sessions/logout.
- Add isolated workspaces, owner/admin/member roles, last-owner protection, and an administrative `bootstrap-owner` command.
- Add expiring, revocable, one-time invitations bound to the target GitHub numeric ID.
- Record transactional audit metadata and enforce server-side role policy.
- Add real-PostgreSQL integration checks, including a mocked GitHub provider exercised over HTTP.

No agent credentials or MCP tools are introduced. Browser management is completed in change 5; login/logout and authenticated workspace-selection pages are introduced here. Without complete GitHub configuration, serving remains in documented operations-only mode with identity routes unavailable.

## Capabilities

### New Capabilities

- `human-authentication`: GitHub identity, secure sessions, state/PKCE, logout, and safe redirects.
- `workspace-membership`: invitation-only membership, role authorization, isolated workspaces, bootstrap, and last-owner protection.
- `workspace-invitations`: targeted, one-time, expiring, revocable invitations and safe acceptance.

### Modified Capabilities

- `service-runtime`: permit configured identity routes while keeping MCP and future management features unavailable.

## Impact

Add an identity/workspace schema migration and Go identity/security/workspace packages; extend HTTP route registration and configuration; add bootstrap CLI and integration tests. Continue using the existing pgx pool, bounded HTTP runtime, embedded SQL, and server-rendered approach. No new infrastructure or frontend toolchain is needed. The user already approved the architecture and authorized autonomous implementation of these sequential changes.
