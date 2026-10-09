# Tasks

The user approved autonomous implementation of the requested MVP. Source publication/CI targets the existing public repository; paid resources and Railway publication remain outside authorization. Preserve AGENTS.md.

## 1. Complete Browser Management

- [x] 1.1 Add escaped shared server-rendered home/workspace layouts, navigation and embedded CSS without a frontend toolchain; verify actual HTML/routes, authenticated isolation, accessible forms and unchanged AGENTS.md.
- [x] 1.2 Add member/role/removal forms and protected handlers using current session/core policy; verify owner/admin/member views, owner-only grants, server rejection of forged role/CSRF submissions, and last-owner behavior over HTTP.
- [x] 1.3 Add invitation create/list/cancel/accept pages and one-time link handling through login without server plaintext persistence; verify intended/wrong-account acceptance, expiry/replay/cancellation, secret-free listing and no-referrer/no-store.
- [x] 1.4 Add owned agent/credential lists and registration/deactivation/issue/revoke/rotation forms plus administrator team-agent deactivation; verify actual HTTP workflows, one-time token pages, foreign credential denial, and no administrative inbox content.

## 2. Request and Deployment Boundaries

- [x] 2.1 Implement pre-body per-peer login/MCP admission and bounded limiter state, plus post-auth per-agent aggregate limits; verify deterministic burst/refill, memory/expiry, 429/Retry-After, shared identity, forwarding-header spoof resistance, no rejected writes, and unaffected health. Document per-replica/peer semantics.
- [x] 2.2 Verify direct/proxy Host and Railway start/predeploy/health configuration against official docs/schema and simulated container entrypoint; retain SDK protection or introduce only an equivalently tested configured policy. Update complete domain/TLS/OAuth/secret/migration runbooks without deploying.

## 3. Full Acceptance and Clients

- [x] 3.1 Add complete mock-GitHub browser-to-MCP acceptance for two invited user identities, real forms/agents/credentials and independent SDK sessions; verify exchange, workspace/inbox isolation, CSRF, revocation and restart persistence without a direct-only shortcut.
- [x] 3.2 Verify installed Codex/Claude versions and bearer configuration using official sources/CLI and isolated temporary settings; record exact commands/results and distinguish parsing/connection from unexecuted model-driven calls. Keep public examples secret-free.
- [x] 3.3 Execute a local PostgreSQL backup and isolated restore drill with synthetic message/credential state; verify restored readiness, ledger, messages/idempotency and active/revoked credentials, then remove only disposable target/dump. Document operator-owned production backup settings.

## 4. Release Proof

- [x] 4.1 Run complete local formatting/module/vet/tests/race/build/module-vulnerability/OpenSpec checks and independent clean-checkout/container/request-reply reproduction; record authoritative outcomes and known external limits.
- [x] 4.2 Verify remote ancestry, review all staged source for real credentials/data, publish to the requested existing public repository, and monitor actual required GitHub Actions until green for the source commit. Do not overwrite divergent remote work or deploy Railway.
- [x] 4.3 Complete original-brief/spec-scenario audit and final README/CONTRIBUTING/SECURITY/client/operations evidence; verify every required deliverable against current files/runtime/CI and leave no unproven local work. Preserve external deployment limits and AGENTS.md bytes.

## Requirement-to-Check Mapping

| Requirement | Tasks |
| --- | --- |
| Authenticated navigation and member administration | 1.1, 1.2 |
| Invitation management and identity-bound acceptance | 1.3 |
| Owned agents and one-time credential pages | 1.4 |
| Private escaped browser rendering | 1.1, 1.4, 3.1 |
| Bounded per-peer admission; Agent request rate; Replica and proxy semantics | 2.1 |
| Complete browser to MCP acceptance | 3.1 |
| Verified client and activation documentation | 3.2 |
| Backup restore and Railway preparation | 2.2, 3.3 |
| Public source and CI evidence | 4.2 |
| Full fresh checkout reproduction and audit | 4.1, 4.3 |

## Workflow follow-up

- Sync and archive only after every tracked task and requirement/scenario passes.
- Publish final archive/evidence commit and verify its GitHub Actions result.
- Do not provision paid resources or publish Railway without explicit user permission; label live deployment unverified.
