# Tasks

> Superseded by `architect-led-orchestration`. The six completed foundation tasks remain historical evidence; unchecked tasks are not completed and are reconciled into the new architecture.
## 1. Durable Delegation

- [x] 1.1 Add additive tenant-scoped task/attempt/runner-ownership schema and models; verify migration repeat/concurrency/checksums, composite references and all prior mailbox tests.
- [x] 1.2 Implement explicit scoped/idempotent submission with request-message atomicity, input/deadline/result limits and canonical fingerprints; verify concurrent retries, changed payload conflict, foreign/forged references and rollback; document the task model.
- [x] 1.3 Implement exclusive agent ownership, recipient claim, renewable 60-second leases and attempt fencing; verify competing runners, expiry, stale completion and credential revocation with deterministic real-PG concurrency tests.
- [x] 1.4 Implement durable result/reply commit, cancellation, expiry and interrupted/retry-safe policy; verify late-result denial, no early ACK, result/message rollback and safe audit; document retry and side-effect semantics.
- [x] 1.5 Implement dependency correlation, inherited deadlines, ancestor-cycle/depth/child limits and waiting status; verify A-to-B-to-C success, A-to-B-to-A rejection and access termination.
- [x] 1.6 Add seven authenticated strict-schema task MCP tools using the official transport; verify two independent HTTP sessions, byte limits, stable errors, foreign participant denial and unchanged existing five mailbox tools; publish exact schemas.

## 2. Local Supervisor and Bridge

- [x] 2.1 Add runner configuration, doctor/start-task/status/serve commands, state-directory locking and protected local control; verify duplicate-supervisor denial, invalid policy rejection, state permissions and token-free arguments/logs; document local setup.
- [x] 2.2 Add ownership/heartbeat/claim/renew loops, bounded backoff and serialized single-slot execution; verify presence/lease tests plus independent supervisor processes and enforced one-active-task capacity.
- [x] 2.3 Add atomic local task/session/result/continuation journal and process-tree shutdown; verify crashes before/after result commit, delivery replay, cancellation, lease loss and explicit unsafe-interruption review.
- [x] 2.4 Implement official SDK local stdio MCP delegate_task/identity/directory bridge with per-job local binding and stable submission keys; verify genuine tool submit/wait/result delivery, spoof denial and long work through short remote calls.
- [x] 2.5 Implement deferred result continuation into the exact originating session after provider timeout/restart; verify original-context preservation, no unrelated-session selection, nested dependencies and controlled uncertainty; document the managed-session workflow.

## 3. Provider Adapters

- [x] 3.1 Add common provider capability/result/error/session/process interfaces and bounded private input/output handling; verify malformed events, overflow, fixed paths and denied permission elevation with executable fake providers.
- [x] 3.2 Add Codex CLI start/exact-resume adapter and local bridge configuration; verify installed supported CLI version/authentication, synthetic real execution plus delegation continuation, permission profile and no prompt/token argv leakage; record exact tested commands.
- [x] 3.4 Install/check official GitHub Copilot CLI and add programmatic JSONL/exact-session adapter with private input file or supported stdin and explicit MCP timeout; verify synthetic real execution/continuation/delegation, authentication and permission profiles; record supported version and distinguish the VS Code wrapper.
- [x] 3.5 Verify Codex and Copilot adapters preserve prior context across two turns and normalize failure/permission/interruption; test Codex-to-Copilot and Copilot-to-Codex through independent runner processes; classify deterministic CI versus real-model evidence explicitly.

## 4. Task Visibility and Distribution

- [x] 4.1 Add participant-owner task list/detail/status/result pages and cancel/review controls; verify actual browser ownership/CSRF/escaping/no-store, unrelated-admin privacy, offline status and token/body-free audit; document visible lifecycle.
- [x] 4.2 Package runner binaries and configuration examples for macOS/Linux, with foreground operation and explicit auto-start install/uninstall instructions; verify clean-checkout build/doctor/configuration and preserve AGENTS.md.
- [x] 4.3 Update README/client/task/operations documentation for one runner per physical machine, local projects/accounts, provider permissions, task/result continuation and retained mailbox example; verify commands and links against current binaries.

## 5. End-to-End Delivery

- [x] 5.1 Run complete formatting/module/vet/tests/race/build/vulnerability/OpenSpec gates including real PostgreSQL and two independent runner/bridge/provider processes; verify regression, restart, fencing, cancellation and A continuing with B's actual result.
- [ ] 5.2 Publish reviewed source and confirm terminal green GitHub CI for its exact commit; deploy additive server support to the authorized Railway project and verify task authentication/readiness and existing mailbox behavior.
- [ ] 5.3 Configure one runner on each of two accessible physical machines and prove a real delegated task, recipient execution and originating-session continuation over Railway; record provider versions/project/account boundaries and actual result evidence. Keep this task incomplete if second-machine access or provider authentication is unavailable.
- [x] 5.4 Audit every task-delegation/machine-runner/task-observability requirement and modified mailbox scenarios against current source, CI and live evidence; document known external limits and unchanged AGENTS.md before synchronization/archive.

## Requirement-to-Check Mapping

| Requirements | Tasks |
| --- | --- |
| Explicit scoped task submission; Idempotent bounded tasks | 1.1, 1.2, 1.6 |
| Exclusive fenced execution; Current participant authorization | 1.3, 1.6, 2.2 |
| Durable completion and automatic continuation | 1.4, 2.3, 2.4, 2.5, 3.5, 5.3 |
| Cancellation and uncertain interruption | 1.4, 2.3, 3.1, 5.1 |
| Bounded delegation dependencies | 1.5, 2.5 |
| One supervisor per machine; Local execution configuration | 2.1, 2.2, 4.2 |
| Two provider adapters | 3.1, 3.2, 3.4, 3.5 |
| Automatic task execution and context continuity; Local delegation bridge | 2.4, 2.5, 3.5, 5.3 |
| Bounded supervision and recovery; Local secret and output handling | 2.1, 2.2, 2.3, 3.1, 3.2, 3.4 |
| Participant-owned task views; Execution status and presence; Protected task controls and safe audit | 4.1, 2.2, 1.4 |
| Modified executable demonstration/activation documentation | 4.3, 5.1, 5.3 |
| Complete verified publication and physical-machine delivery | 5.1, 5.2, 5.3, 5.4 |

## Workflow follow-up

- Sync and archive only after every tracked task and scenario has authoritative passing evidence.
- Publish final archive/evidence and verify its exact commit's GitHub Actions.
- Claude adapter and Claude-inclusive cross-provider evidence were split into `claude-adapter-followup`.
