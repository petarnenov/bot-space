# Proposal

## Why

Verified GitHub membership now provides a trustworthy owner/team boundary. Each registered agent needs its own independent credentials before the MCP mailbox can safely derive sender and workspace identity.

## What Changes

- Add workspace/user-owned agents, registration, own-agent listing, and authorized deactivation.
- Add high-entropy per-agent bearer tokens shown once, stored only as hashes, with revocation and atomic rotation.
- Add uncached credential authentication and transaction-bound rechecking of owner membership and agent activation.
- Extend membership removal to deactivate agents and revoke their credentials atomically, including across later rejoining.
- Add safe transactional audit metadata, real-PostgreSQL/HTTP authentication checks, and credential documentation.

No MCP tools, message storage, full management UI, OAuth authorization server, or automatic agent activation is introduced. Change 4 mounts the verified authentication boundary around the official SDK endpoint; change 5 completes credential forms.

## Capabilities

### New Capabilities

- `agent-registration`: scoped ownership, member registration/listing, deactivation policy, and lifecycle audit.
- `agent-credentials`: one-time bearer issuance, hashes, rotation/revocation, and uncached identity validation.

### Modified Capabilities

- `workspace-membership`: agent access termination on removal, preserving historical agent records and preventing credential revival after rejoining.

## Impact

Add `0003_agents_credentials.sql`, a Go agents service/authenticator, membership-removal revocation, and integration tests. Keep the existing browser identity and role policies. No infrastructure or dependency changes are needed. The public module remains `github.com/petarnenov/bot-space`.
