# Tasks

## 1. Council rules

- [x] 1.1 Implement the pure fixed-council majority engine with immutable proposals, one vote per member, three rounds and material reconsideration; verify odd/even thresholds, stale/outsider votes and exhaustion tests, and document the API.

## 2. Native control transport

- [x] 2.1 Define protobuf control messages and generated gRPC services with bounded unary/stream behavior; verify local authenticated unary calls, trailers and simultaneous bidirectional tests and document generation.
- [x] 2.2 Prove native TLS gRPC through the real Railway ingress, including trailers and reconnect; retain reproducible client output and document the verified topology.

## 3. Automatic collaborator identity

- [x] 3.1 Implement owner/explicit-collaborator GitHub verification with immutable identity, bounded cache and fail-closed refresh; verify public-reader denial, outage and collaborator-removal tests and document required server configuration.
- [x] 3.2 Implement startup OAuth enrollment bound to runner key with role selection, credential issuance/refresh/revocation and no invitations; verify key/replay/role/open-stream revocation tests and document startup.
- [x] 3.3 Support separate architect and executor identities/state on one host and multiple eligible projects per machine role; verify concurrent startup, scoped credential isolation, stable cross-project machine/GitHub identity, sockets and journals and document both commands.

## 4. Human backlog and OpenSpec contracts

- [x] 4.1 Add project/backlog migrations and authenticated human intake with immutable provenance, idempotency and revisions; verify real-PG restart, cross-project and runner-impersonation tests and document intake.
- [ ] 4.2 Add versioned OpenSpec contracts, artifact hashes, task/scenario references and Git base revisions; verify stale/missing-contract rejection and scope-provenance tests and document the contract format.

- [ ] 4.3 Implement human-creator pause/resume/cancel/archive with immutable lifecycle history and root execution fencing; verify authorization/CSRF, concurrent actions, cancelled-root revival denial, preserved archived evidence and architect reconciliation before resume, and document controls.

## 5. Durable council and control events

- [ ] 5.1 Persist council snapshots, rounds, votes and accepted commits transactionally with authorization; verify concurrent coordinator/vote races and material reconsideration in real PG and document state transitions.
- [ ] 5.2 Implement durable outbox, runner cursors, local ACK-after-persist and deduplicated replay; verify disconnect/restart around acknowledgement and bounded backpressure tests and document delivery guarantees.
- [ ] 5.3 Implement architect proposal/discussion/voting sessions and coordination without human intervention; verify fresh seeded sessions per decision, exact-session retention across rounds, isolated parallel questions, independently authenticated councils, missing members, ties and three-round blocking with provider fixtures and document the runtime.

## 6. Single-task executor allocation

- [ ] 6.1 Implement atomic majority-backed assignment with exactly one reserved task per executor across providers/projects, including waiting questions and uncertain interruptions; verify cross-project/cross-connection concurrent assignment and occupied-slot tests and document availability.
- [ ] 6.2 Remove execution deadlines, inherited budgets and duration-based provider cancellation while retaining renewable authority leases; verify healthy execution beyond former limits and interrupted-authority reconciliation tests and document the distinction.
- [ ] 6.3 Implement question decisions bound to contract/attempt/session and exact continuation after accepted answers; verify fresh sessions between tasks, retained context for same-task questions/review, stale answers, reconnect and occupied-slot behavior and document the local bridge.
- [ ] 6.4 Implement artifact/evidence return, council review and safe retry/reassignment; verify late-result fencing, uncertain side effects and failed-evidence rejection without human approval and document recovery.

## 7. Local client adapters

- [ ] 7.1 Implement startup provider/model/effort selection with CLI precedence, durable effective settings and capability validation; verify omitted settings preserve client defaults, live catalog/model-specific effort choices, invalid settings start no work session, command routing and exact resume settings and document usage.
- [ ] 7.2 Finish Codex and Claude adapters and real authenticated exact-session smoke checks; verify structured results, local permissions, credential separation and architect-question continuation and document tested versions.
- [ ] 7.3 Finish Copilot adapter on machine 192.168.1.223 through VPN; verify authenticated execution, selected settings, exact-session continuation and returned evidence and document tested capabilities.
- [ ] 7.4 Implement Hermes adapter from its verified installed CLI/API contract; verify capability negotiation, startup settings, structured results and exact-session continuation and document reproducible installation/smoke evidence.
- [ ] 7.5 Implement OpenClaw adapter from its verified installed CLI/API contract; verify capability negotiation, startup settings, structured results and exact-session continuation and document reproducible installation/smoke evidence.

## 8. Autonomous integration and UI

- [ ] 8.1 Implement one OpenSpec branch/executor worktree per change, per-task commits/pushes with immediate architect notification and publication-pending recovery, and separate architect review worktrees fetched at the exact executor head, plus evidence-gated council acceptance, merge/deploy and OpenSpec synchronization/archive within configured authority; verify failing-check publication denial, push retries/remote conflicts without force-push, stale-head approval, dirty worktree preservation, changed main, failed checks and missing permissions block actions and document integration policy.
- [ ] 8.2 Implement human intake/backlog and council/assignment/question/evidence/executor views with project isolation; verify rendering, CSRF, safe audit and absence of approval/override controls and document UI usage.

## 9. Durable agent learning

- [ ] 9.1 Implement idempotent evidence-backed lesson candidates/revisions with project authorization and safe audit; verify real-PG restart, duplicate capture, cross-project isolation and bounded content tests and document lesson records.
- [ ] 9.2 Implement majority-backed lesson promotion, explicit generalization and revision/revocation with correction-evidence gates; verify stale votes, missing evidence, rejected lessons and contradictory evidence tests and document transitions.
- [ ] 9.3 Seed fresh architect/executor sessions with bounded applicable accepted lessons and record exact supplied revisions; verify rejected/revoked/unrelated lessons are excluded, budget limits and fresh-session isolation and document retrieval.
- [ ] 9.4 Implement recurrence tracking and architect-approved revised mitigation before repeating known failure; verify a reproduced mistake links its supplied lesson and prevents blind retry, and document learning feedback.

## 10. Delivery verification

- [ ] 10.1 Verify HUMAN intake through planning, majority allocation, executor question/answer/resume, evidence review and authorized integration without human gates; run real-PG end-to-end/restart tests and preserve scenario evidence.
- [ ] 10.2 Run repository gates and hosted CI, publish authorized source and deploy additive migrations/services; verify legacy mailbox compatibility, readiness and public native control with factual output.
- [ ] 10.3 Verify physical Mac architect plus Copilot executor on 223 and same-host dual-role startup; record topology, long-work single-slot behavior, reconnect and final reviewed artifacts without claiming simulated machines are physical.

## Workflow follow-up

- Reconcile the superseded distributed-agent-runner plan without marking unfinished tasks complete; preserve its verified foundation evidence.
- Archive only after every requirement and delivery gate has factual evidence; keep AGENTS.md unchanged.
