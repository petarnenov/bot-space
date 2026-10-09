# MVP Verification

Verified locally on 2026-10-09 (Europe/Sofia) with Go 1.27.2, PostgreSQL 18.6,
official MCP Go SDK v1.8.0, Docker 29.5.2, and OpenSpec 1.14.1.
Every identity, credential, message, and backup fixture used synthetic data.
Historical archive reports describe their stage; current files describe the MVP.

## Executed Gates

Formatting, module verification, vet, ordinary tests, race tests, build,
module-level govulncheck, and strict OpenSpec validation passed.
PostgreSQL tests ran with TEST_DATABASE_URL; the restore drill ran with
PG_BACKUP_CONTAINER. govulncheck v1.8.0 reported no vulnerabilities.
Exact repeatable commands are in README.md.

An independent clone of source commit dd4677331e201200e52d870d06e309a6cb78fa49
used a new Compose project, volume, and loopback ports 18081/15433. Docker
build/start/readiness 200, module verification, vet, full race suite, build, and
OpenSpec validation passed. The integration package passed in 48.458 seconds,
including browser OAuth, SDK exchange, executable clients, restart persistence
and restore. Cleanup removed only that disposable stack/volume. The checkout
had no tracked modifications.

The new image also ran explicit migration and direct /mailbox serve with
PORT=8091; readiness was 200 and runtime UID was 65532:65532.
Railway TOML matches its freshly downloaded official schema.
No Railway resources or deployment were created.

## Original Brief Audit

| Requested deliverable/boundary | Current evidence |
| --- | --- |
| Five sequential OpenSpec changes, approval before implementation, pinned versions and English source/docs | Change artifacts, foundation approval record, tool versions, module locks and image digests. |
| Independent Go mailbox, official Streamable HTTP /mcp, PostgreSQL and minimal Go HTML | cmd/mailbox and internal/mcpserver/mailbox/web; official SDK handler; no custom transport, broker or frontend build. |
| GitHub-only immutable identity, state/PKCE/redirect protection, minimum scopes, protected sessions/logout, discarded provider tokens | Mock provider HTTP and session tests; fixed provider URLs, no requested scopes; hashed session/OAuth state and no provider token persistence. |
| Multiple workspaces/roles, invitation-only membership, targeted one-time expiring/cancellable invitations, bootstrap and last owner | Workspace/invitation tests, concurrent owner/acceptance tests, actual protected forms and bootstrap CLI; identity runbook. |
| Scoped human-owned agents, administrator deactivation, random separate hashed one-time credentials, rotation/revocation and per-request checks | Agent domain/browser/HTTP tests, actual transaction lock checks, audit rollback, removal trigger and rejoining tests. |
| Credential-derived sender/workspace, cross-workspace and own-inbox/ACK isolation, no human administrative inbox privilege | Native SDK spoof/isolation/reference tests and browser-to-MCP acceptance; admin views checked for absent message bodies/tokens. |
| Five tools, explicit schemas, payload/result limits, stable errors and validated references | SDK tools/list/call tests, validation/cursor tests, MCP documentation; actual serialized SDK results measured at most 256 KiB. |
| Committed durable/offline delivery, idempotency/conflict/rotation, nondestructive reads, repeated ACK | Real-PG rollback/concurrent retry tests; real process restart and HTTP reads; restored idempotency and credential state. |
| Concurrent ordering/pagination, shared identity and retention without false guarantees | Delayed-commit counter lock test; signed snapshots/live ACK; same-identity clients; no deletion/claim/lease; indefinite retention and duplicate processing documented. |
| Separate runnable clients, provider-neutral activation and external-data semantics | examples/requestreply runs as two OS processes; helper rejects token forwarding/redirects; documented explicit polling and future runners/webhooks/push. |
| All requested browser pages, authorization/CSRF and administrative privacy | Complete browser acceptance and management lifecycle/role tests; foreign credential/CSRF denial, escaped templates, embedded CSS and one-time token pages. |
| HTTP limits/timeouts/rates/Origin, safe logs/audit, graceful shutdown/pooling/migrations/health | Unit/HTTP/real-PG tests for boundaries, slow headers, Origin, every-method auth, log sentinels, atomic audits, SIGINT/SIGTERM, pool, migration integrity and readiness. |
| Environment-only secrets, ignored local files and placeholders | Config/startup sanitization tests, .env.example, .gitignore, staged-source review and ignored-file checks. |
| Railway preparation, non-root container, injected all-interface port, DB URL, migration/health, durable Compose and operations | Official schema, real container commands, lifecycle proofs, Host policy tests, backup/restore drill and domain/TLS/OAuth/secret/backup runbooks. |
| Requested public repository, MIT, README/CONTRIBUTING/SECURITY and checked clients | Fast-forward source publication to petarnenov/bot-space; complete documentation; installed Codex parser and Claude authenticated-connection tests. |
| Local/hosted gates and clean-checkout reproduction | Local gates and independent checkout passed; Actions includes real PG and module vulnerability scan. Hosted terminal result is recorded below before archival. |
| Preserve AGENTS.md | Byte comparison with original commit da7285d; unchanged. |

## Final Change Requirements

| Requirement | Executed evidence |
| --- | --- |
| Authenticated navigation and member administration | Owner/admin/member controls, forged escalation, last-owner demotion/removal, successful role/removal, removed-member page denial and CSRF tests. |
| Invitation management and identity-bound acceptance | Full mock-login flow carries invite through login; wrong account fails without consumption; HTTP cancellation/expiry/replay, secret-free listing and no-store/no-referrer tests. |
| Owned agents and one-time credential pages | Browser register/issue/rotate/revoke/deactivate; foreign owner issuance denied; subsequent views contain metadata only. |
| Private escaped browser rendering | HTML/CSS and administration privacy tests; no inbox bodies in views. |
| Bounded per-peer admission | Deterministic burst/refill/cap/expiry; rejection before body consumption; login/MCP quotas and health/readiness bypass. |
| Agent request rate | Separate credentials/clients share one identity quota, other agents remain independent, refill works, rejected sends write no rows, revoked credentials denied before quota. |
| Replica and proxy semantics | Spoofed forwarding headers cannot bypass; one replica; actual socket peer and independent replica quotas documented. |
| Complete browser to MCP acceptance | Both mock GitHub users use real invitation/agent/credential forms and independent SDK send/read/ack/reply/read; foreign/revoked access denied; real-process test proves offline persistence. |
| Verified client and activation documentation | Codex 0.162.0 parses bearer configuration; Claude 2.1.292 reports Connected using isolated settings and environment bearer; Go SDK processes exchange. No model-driven claim. |
| Backup restore and Railway preparation | Actual pg_dump/pg_restore verifies schema/message/idempotency/active and revoked keys; official schema, real container and direct/proxy Host tests pass. |
| Public source and CI evidence | Reviewed source fast-forward published; required hosted gates must reach green before archival. |
| Full fresh checkout reproduction and audit | Clean clone checks and current original-brief/spec review; complete index below; external limits retained and AGENTS.md unchanged. |

## Specification Coverage

The [requirement/scenario index](spec-coverage.md) names every current requirement
and scenario and its evidence files. Archived change reports retain test-specific
explanations. Those tests passed again on current source. Historical private-route
unavailability applies only to unconfigured modes or the earlier stage.

## Hosted CI Evidence

The source candidate dd4677331e201200e52d870d06e309a6cb78fa49 was published by
fast-forward over the existing foundation commit. Its
[GitHub Actions run](https://github.com/petarnenov/bot-space/actions/runs/37861164754)
completed successfully. Formatting, module checksums, vet, real-PostgreSQL tests,
race detector, build, module vulnerability scan, and strict OpenSpec validation
each report success. The workflow contains no deployment step.

The final archive/documentation commit repeats the workflow; verify its result
against its exact SHA in repository Actions. The source candidate's green result
does not itself prove a later commit's CI outcome.

## External and SDK Limits

Live GitHub OAuth with production credentials, Railway proxy/TLS/domain
deployment, and managed production backup schedules remain unexecuted.
Codex proves parsing; Claude proves authenticated connection. Neither invokes a
model. Native SDK and two-process exchanges are executed. Bearer mode is not
full MCP OAuth.

Unmodified SDK v1.8.0 can reject extreme JSON exponents during protocol header
extraction; tests require exact acceptance or explicit rejection with no write.
Normal large integers remain exact. There is no exactly-once, single-consumer,
auto-wakeup, distributed quota or automatic pruning guarantee. Database operators
can access stored data and backups.
