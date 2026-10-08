# Tasks

The user approved the architecture and authorized sequential implementation. All tasks are verified complete; see `verification.md`. All mailbox specifications precede code. Direct service tests supplement, but never replace, real HTTP MCP and executable two-process checks.

## 1. Persistence, Validation, and Cursors

- [x] 1.1 Add tenant-consistent conversations/messages/inbox-counter migration and message types; verify migration/repeat/integrity tests against PostgreSQL and field/constraint behavior.
- [x] 1.2 Implement byte/depth/type/UUID validation, exact JSON metadata preservation, and canonical payload fingerprints; verify boundary cases, NUL/invalid UTF-8, large integers, number/key-order/default equivalence, and meaningful-payload differences.
- [x] 1.3 Add environment cursor-key/origin configuration and scoped HMAC cursors; verify absent/partial/invalid configuration, tampering, wrong purpose/agent/workspace/filters, and consistent cursors across a reconstructed signer. Document setup and key-change effects.

## 2. Durable Delivery and Inbox Processing

- [x] 2.1 Implement transactionally rechecked send, eligible recipient locks, participant threads/replies, and sender/key idempotency; verify different-owner delivery, invalid/inactive/foreign recipients, invalid/invisible references, concurrent same-key retries/conflicts, token rotation and committed retries after recipient deactivation.
- [x] 2.2 Implement per-inbox commit-order counters, own-inbox reads, signed snapshot/byte-limited pages, and idempotent own acknowledgement; verify repeated reads, sender/nonrecipient denial, delayed concurrent commit ordering, inserts between pages, acknowledgement filter changes, cursor misuse, result size limits, and shared-identity reads.
- [x] 2.3 Verify transaction rollback and offline/restart persistence for messages/threads/counters/fingerprints; document indefinite retention and no exactly-once/single-consumer guarantee, with evidence from failed-insert rollback and application replacement.

## 3. Official MCP Boundary

- [x] 3.1 Lock official Go SDK v1.8.0 and register the five tools through SDK Streamable HTTP with explicit schemas/strict arguments/stable bounded JSON text results; verify tools/list schemas and all five calls through independent HTTP SDK clients.
- [x] 3.2 Mount uncached bearer and exact Origin protection, SDK/global body limits, cancellation settings and discard-only SDK logging; verify unauthorized/cookie-only/revoked requests, bad/null/duplicate origins, absent native Origin, oversized chunked input, spoofed parameters, private error/log sentinel exclusion, and SDK protocol behavior.
- [x] 3.3 Integrate optional mailbox configuration in the real binary independently of browser identity; verify operations-only MCP 404, configured MCP authentication, additive migration readiness and container restart behavior, and document actual route/configuration semantics.

## 4. Executable Clients and End-to-End Proof

- [x] 4.1 Add a bounded official SDK client helper and separate requester/responder executable roles with environment credentials, peer IDs, shared run ID, one-second polling, and origin-restricted/no-redirect credential forwarding; verify client configuration errors and actual two-process request/reply without token/body logging. Document multi-machine invocation and inactive-agent limitations.
- [x] 4.2 Add required real-PostgreSQL HTTP MCP end-to-end test using two distinct user-owned agent sessions: send → read → acknowledge → reply → read; verify workspace/inbox/reference isolation, idempotency, concurrency, payload/result limits, revocation/removal, multiple same-identity clients and persistence after reconstructing/restarting the application.

## 5. Integrated Verification

- [x] 5.1 Include examples in formatting/build/CI coverage and run module verification, vet, full tests/race with PostgreSQL, build, vulnerability scan, and strict OpenSpec validation; record every requirement/scenario with actual evidence in `verification.md` and distinguish unexecuted external deployment/client checks.
- [x] 5.2 Build and smoke-test the real configured container and executable example, including offline delivery through application replacement while preserving PostgreSQL; verify previous foundation/identity/credential behavior and keep publication/deployment unexecuted.

## Requirement-to-Check Mapping

| Requirement | Tasks |
| --- | --- |
| Official authenticated remote transport; Explicit mailbox tool schemas | 3.1, 4.2 |
| MCP Origin and body protection | 3.2 |
| Credential-derived identity and recipient directory | 3.1, 4.2 |
| Tool limits; Stable application failures | 1.2, 2.2, 3.1, 3.2 |
| Executable two-client request reply | 4.1, 4.2 |
| Committed durable messages; Explicit retention | 2.3, 5.2 |
| Credential-scoped sender and eligible recipient; Participant-scoped threads and replies; Sender-scoped idempotency | 2.1, 4.2 |
| Metadata fidelity and external-data semantics | 1.2, 2.1, 4.1 |
| Own inbox and nondestructive reads; Idempotent own acknowledgement | 2.2, 4.2 |
| Commit-ordered inbox sequences; Signed snapshot pagination; Live acknowledgement filtering; Shared identity and repeated processing | 1.3, 2.2, 4.2 |
| Modified runtime availability | 3.3, 5.2 |

## Workflow follow-up

- Sync/archive only after every task and requirement/scenario is verified.
- Create `web-management-readiness` proposal/specs/design/tasks before final management UI, client/runbook and deployment-readiness work.
