# Architect Led Orchestration

## Why

The user clarified that bot-space must autonomously execute HUMAN-submitted OpenSpec work through solution-architect and executor runners. Peer message delegation does not provide a shared backlog, architect decisions, executor capacity or the required majority consultation.

## What Changes

- HUMAN creates top-level stories/tickets/descriptions in a project queue and remains outside planning, assignment, clarification, review and integration loops.
- A machine may run one architect supervisor and one executor supervisor concurrently, each with separate identity/state and GitHub authentication at startup. The server automatically verifies repository ownership or explicit collaborator access, issues scoped machine credentials and connects over gRPC without a bot-space invitation or manual role grant.
- Two registered runner roles: architect and executor. Codex, Claude Code and Copilot are local provider adapters rather than holders of central credentials.
- Registered architects decide by strict majority of a fixed council: floor(N/2)+1. Ties continue discussion. At most three rounds, with no minute limit; unresolved decisions wait for new information without HUMAN approval or overrides.
- Executors advertise capabilities and availability; each executor handles exactly one active task, including while awaiting answers. The server atomically reserves its single slot and records fenced assignments based on approved council decisions.
- Execution has no elapsed deadline or inherited time budget. Long-running healthy work remains active; technical connectivity and renewable lease checks do not expire tasks because of duration.
- Executor questions trigger a separate council decision and resume the exact execution session after an authoritative answer.
- Work packages refer to versioned human intent, OpenSpec artifacts, Git base revision and acceptance evidence. Architect decisions govern scope revisions, retries, review, merge and deployment within configured project authority.
- Add durable evidence-backed lessons for architects and executors, with majority promotion, scoped retrieval for fresh sessions and recurrence tracking.
- Add backlog/council/capacity/question/evidence views with explicit project permissions.
- Preserve the existing mailbox and reuse verified transaction/idempotency/lease foundations. The old distributed-agent-runner plan is superseded, not completed or archived.

## Capabilities

### New Capabilities

- human-work-queue: Human-originated project intentions and approved-scope derived work.
- runner-control: Typed gRPC control, durable event delivery and reconnect behavior.
- runner-identity: GitHub enrollment, machine credentials and authorized runner roles.
- architect-council: Version-bound majority decisions and bounded discussion without human intervention.
- work-allocation: Capability matching, capacity reservation, fenced execution and architect-authorized acceptance.
- spec-governed-work: OpenSpec/repository revisions, evidence and autonomous integration.
- project-observability: Project-authorized backlog, council and execution visibility.
- agent-learning: Durable validated lessons, task/decision context retrieval, recurrence evidence and lesson revision.

### Modified Capabilities

- None initially. Existing mailbox remains compatible; new control services are additive.

## Impact

Go server/migrations, runner executable, provider adapters, protobuf/gRPC dependencies, browser management and CI/integration tests. GitHub sessions identify humans; new orchestration admission requires repository ownership or explicit collaborator access rather than legacy workspace invitations. Additive role authorization does not grant public workspace joining or expose unrelated inboxes.

Transport support through the current Railway ingress must be proven, including native gRPC trailers and bidirectional streaming. Alternative ingress requires explicit evidence and a documented decision; an HTTP/2 label alone is insufficient.

The user authorized sequential implementation and live delivery. No extra planning approval is requested for these confirmed decisions. Choose routine implementation details and document them. Keep AGENTS.md unchanged, secrets local and unsupported provider/physical-machine claims unverified.
