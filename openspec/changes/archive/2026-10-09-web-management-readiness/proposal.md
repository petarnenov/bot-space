# Proposal

## Why

The four verified backend changes now need a complete usable management surface and final operational proof. Contributors and operators must reproduce the full private mailbox, configure real clients safely, and prepare Railway without claiming an unexecuted deployment.

## What Changes

- Complete server-rendered login/workspace/member/role/invitation/agent/credential pages, with ownership-aware forms and server-side CSRF/authorization.
- Add bounded per-peer login/MCP request limits and per-agent MCP limits, with one-replica semantics and safe HTTP errors.
- Verify proxy/Host behavior, deployment settings, backup/restore and fresh-checkout workflows.
- Add a complete HTTP browser/mock-OAuth-to-MCP acceptance path, client configuration records, and final requirement audit.
- Publish the completed source changes to the existing public `petarnenov/bot-space` repository and verify its required GitHub Actions workflow, after local checks and secret review.

No paid resources, Railway publication, live OAuth app registration, arbitrary agent runners, message-admin UI, or new frontend/services are introduced. Existing public GitHub source already contains the verified foundation commit; repository publication is part of the requested public-source/CI deliverable. Railway deployment remains explicitly unexecuted without separate permission.

## Capabilities

### New Capabilities

- `web-management`: usable private team/agent administration pages, one-time secret display, and invitation acceptance.
- `request-limits`: bounded per-peer/per-agent admission and documented per-replica rate behavior.
- `release-readiness`: complete reproduced acceptance, verified client configuration, backup/restore, public-source CI and Railway preparation evidence.

### Modified Capabilities

None; existing identity, membership, agent, mailbox, and runtime contracts remain in force. The new pages use the established protected mutation services.

## Impact

Add Go-rendered management templates/static CSS, human mutation routes, bounded limiter middleware, full acceptance tests, client/operations documentation and final verification records. Reuse existing core services and official MCP transport. Preserve AGENTS.md as requested. No standalone frontend toolchain is added.
