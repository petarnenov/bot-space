# Architect Led Orchestration Design

## Context

The confirmed workflow replaces direct peer delegation with a human-originated backlog, architect decisions and registered executor capacity. The user requires GitHub authentication at runner startup, automatic collaborator-only access without bot-space invitations, autonomous architects, strict majority voting, and at most three rounds without minute limits.

Existing Go/PG infrastructure, identity sessions and verified legacy task transaction/lease behavior provide a foundation. The unfinished distributed-agent-runner supervisor/provider code is reusable only after review; its per-agent credentials, human review state and direct-delegation workflow do not satisfy this change. Preserve its completed-task evidence and do not archive it as finished.

## Goals and Non-Goals

Deliver one machine supervisor with an architect or executor role, local Codex/Claude/Copilot adapters, native gRPC control, durable OpenSpec work and a project dashboard. Machine 192.168.1.223 uses Copilot; the local Mac provides the initial architect.

Keep existing mailbox data/tools compatible. Do not grant access merely because the repository is public, introduce per-task human approvals, interpret transport timeouts as discussion deadlines, fabricate successful checks, or claim exactly-once model/filesystem effects.

## Decisions

### Identity and project authorization

GitHub is the authority for repository ownership and explicit collaborator access. A public reader is not admitted. Owners and verified collaborators may choose either runner role at startup; no bot-space invite or manual role approval is added.

Reuse the existing fixed GitHub provider/state/PKCE protections through a runner startup login flow. The machine starts an enrollment attempt bound to its key, opens the server's GitHub login URL and retrieves its authenticated session after proof of key possession. The callback automatically verifies GitHub access before issuing a scoped credential. Do not distribute the OAuth client secret.

Use repository collaborator verification through a server-side GitHub integration with the required read permissions, rather than inferring access from public repository metadata. Validate immutable GitHub user IDs. An inability to verify fails closed. Cache verified authority for at most 60 seconds and revalidate at refresh; document this removal-detection bound. Never promote a display username or client-provided identity into authority.

Machine credentials are revocable, short-lived and bound to project, owner, runner key and role. Every control mutation checks current credential and authority, including messages on an already-open stream. Local models receive only per-job bridge capabilities, never the central credential. Provider authentication is separate.

Legacy workspace invitation membership remains a mailbox policy; new orchestration project access is repository-derived. Do not accidentally use old invitation checks to reject an authorized collaborator or open all legacy inboxes during automatic admission.

### Control transport and durable events

Use official grpc-go/generated protobuf messages with TLS at the public endpoint. Registration/presence and bounded queries use unary methods; Connect carries sequenced assignments, votes, questions and results over a bidirectional stream. Preserve REST/browser sessions for human intake/UI and local MCP tools for provider integration.

Persist an event/outbox before delivery and receiver ACK only after local durable state. Reconnect resumes from committed cursors, deduplicates event IDs and verifies current credentials. Use bounded frames/queues, flow control and technical deadlines. Long discussion has no elapsed-time approval/closure logic.

Prove unary status trailers and simultaneous bidirectional traffic through Railway before choosing the actual listener/ingress topology. Public HTTP/2 documentation is insufficient. An alternative ingress is not silently substituted for native gRPC; if necessary, produce a concrete reviewable transport option and retain the unfulfilled public transport gate.

### Shared backlog and work contracts

Add project, repository-access, human-intent/revision, OpenSpec-contract, work-package, executor-reservation, attempt, question and evidence records. Keep human source provenance separate from machine-derived work. HUMAN uses authenticated browser intake; runner credentials cannot create root goals.

The common queue is authoritative per project. Intents and contracts are immutable versions; work packages bind root revision, artifact hashes, task/scenario IDs, repository/base commit, allowed actions and acceptance criteria. Scope changes require an architect decision and a new contract, not a human approval gate. Independent business goals require new HUMAN input.

The server owns execution state and enforces invariants. Architects supply proposals and model reasoning; they do not directly declare unsupported transitions successful.

### Majority decision engine

Each decision records kind (plan, allocation, answer, review, retry or integration), material input fingerprint, immutable proposal revision/hash and a fixed list of authorized architect runner IDs. Each member has one vote. Multiple provider sessions on a runner do not create additional identities.

Required support is floor(N/2)+1, including offline members in N. Allowed responses are approve, object and need_information. Votes bind the same proposal hash. A strict majority accepts immediately. All members responding without majority ends that unsuccessful round; a revised proposal starts the next round. The proposal may change within a decision, but each round gets its own immutable content and fresh votes.

At most three rounds are permitted. There is no deliberation duration field, timer, automatic vote or coordinator tie-break. Missing members leave the round waiting; transport reconnect/presence remains independent. After three unsuccessful rounds, blocked_no_majority prevents execution.

Reconsideration needs materially changed source/spec/evidence or an explicit council-membership revision. Formatting changes, new timestamps and cosmetic restart labels do not reset the cap. Preserve a link to the exhausted decision and its material input fingerprint. Council membership snapshots never shrink simply because a machine disconnects.

A leased coordinator organizes draft/revision work and publishes messages; transactional server validation commits majority decisions. HUMAN supplies roots and platform setup but never participates in operational voting or overrides disagreement.

### Capacity, questions and execution

Executors publish provider/version, configured projects, tools/policy and online/busy/draining status. Each has exactly one slot, without configurable parallel execution. Registration is descriptive; assignment atomically checks a matching accepted decision and reserves that slot with a fenced epoch. Waiting for council answers and unresolved interruptions retain the reservation. Local scheduling and server constraints enforce one active package across providers and projects.

A physical machine may host one architect runner and one executor runner concurrently. Give them distinct machine credentials, role-bound identities, private state directories and control sockets. Exclusive state locks apply per runner state directory rather than prohibiting another role on the same host. Both roles share physical CPU/memory; no additional executor slot is created by this arrangement.

Question handling is distinct from execution dependency edges. Executor ask_architects creates a question decision bound to the exact assignment/spec/session. Architect model sessions remain able to answer while other sessions monitor work. An accepted answer resumes the exact executor session; stale answers do not mutate a revised contract.

Leases/fencing stop authoritative stale completion, not arbitrary prior local effects. On uncertain interruption the council reviews evidence and decides a permitted retry; no HUMAN requires_approval gate remains in the new workflow. Missing local authority/capability produces a blocked reason and reassignment/architect resolution.

Execution has no elapsed deadline: remove the legacy 1800-second default, 7200-second cap, parent deadline inheritance and provider contexts derived from task duration. Waiting on questions has no time budget. RPC/heartbeat/lease bounds protect connectivity and authority only; renewable leases support arbitrarily long healthy execution. Disconnects cause interruption/reconciliation rather than duration-based failure. Local bridge calls must acknowledge durable questions and support later answer delivery instead of holding one bounded RPC open throughout heavy work. No inactivity watchdog may silently reintroduce a task deadline.

Use isolated local branches/worktrees and explicit repository mappings. Artifact manifests contain commits/diffs, digests and factual check/scenario evidence. A model's final text or process exit 0 is not sufficient acceptance.

Create one `openspec/<change-id>` branch from main per OpenSpec change, with an executor worktree. Executor commits are pushed to the configured Git remote. Architects fetch the change branch and create/update separate detached review worktrees at the exact submitted commit; do not share or switch the executor's working directory. A clean review worktree may advance only after fetch and explicit commit verification. Preserve dirty worktrees for reconciliation rather than discarding local changes. Review and acceptance bind branch, exact head, contract hash and check evidence. A changed executor head invalidates approval; merge to main requires successful checks and council acceptance, with current-main integration revalidation. Git operations use argument arrays and validated repository/branch mappings.

### Provider and UI integration

Reuse one private state lock, machine key/session journals, process-tree cancellation and protected local control. Adapt the local SDK bridge to ask_architects, report_progress, return_result and architect proposal/vote functions with role-bound job identities. Do not give models raw gRPC credentials or global permission bypass.

Retain all three adapters and verify installed CLI versions, real structured outputs, exact resume and configured permissions. Copilot CLI 1.0.93 was observed on 223; do not infer its model-authenticated success from version output alone.

The web app shows HUMAN backlog/intake, contracts/spec versions, council proposals/votes, executor directory/capacity and assignment/question/evidence timelines. There are no human approval or consensus-override buttons. Project-authorized architects can see required executor context; unrelated projects and legacy inboxes remain private. Preserve escaping, CSRF, no-store/no-referrer and safe audit/logging.

## Risks and Trade-offs

- Ingress incompatibility: real native gRPC proof before declaring Railway control ready.
- GitHub access outages/rate limits: bounded cache, read-only verification integration and fail-closed refresh.
- Missing votes: no minute timeout or invented majority; pending/blocked states are explicit.
- Three-round evasion: material input fingerprints and traced reconsideration.
- Interrupted side effects: fenced authority plus evidence-based architect reconciliation.
- Provider drift: capability/version checks and separate native smoke evidence.
- Different project revisions: contract hashes/base commits and isolated integration workflow.

## Migration Plan

Finish and validate these artifacts before implementation. Mark the old change superseded without deleting evidence or checking incomplete work off. Introduce additive schemas/services behind an orchestration flag, retaining mailbox-only behavior until verification.

Implement/test the pure majority engine, native transport probe, automatic identity/access, backlog/contracts, durable council, capacity and machine-role runtime, provider adapters and UI. Run local real-PG/fake-provider tests, then native-provider and physical-machine proof. Publish source/CI and deploy to the existing authorized Railway project; do not mutate applied migration SQL or overwrite divergent source.

For one architect/one executor, two physical machines suffice. Test multi-member voting through independently authenticated processes and clearly identify physical versus simulated topology. Do not claim three separate physical machines without evidence.

Rollback disables orchestration and stops new assignments while preserving durable records and the existing mailbox. Existing AGENTS.md must remain byte-identical.

### Incremental task publication

For each completed small implementation task, commit and push to the change branch immediately after its relevant checks. Include the OpenSpec task ID and publish commit/check metadata as a control event after Git confirms remote availability. Architects fetch that exact commit into their review worktrees without waiting for the whole change. Keep an idempotent local publication journal: a failed push retains the commit and marks publication pending; retries verify the remote ref and do not create duplicate completion events. Never reset dirty work or overwrite a divergent remote branch. Successful incremental review does not permit main integration before the whole change passes its required gates.

### Executor blocker context

`ask_architects` carries task/contract/attempt/session references, exact branch/commit, attempted approaches, bounded relevant diff/artifact references, factual checks/errors and a precise decision request. Redact credentials/secrets. Preserve the single-slot reservation and exact model session while the council discusses. An accepted version-bound answer resumes that session; no human operational escalation or fabricated completion replaces council resolution.

### Verified ingress constraint and selected transport candidate

A public test on deployment `cf343487-7f56-4aed-98e4-d3c9e27bbbb8` returned HTTP 505. Railway request logs prove HTTP/2.0 downstream but HTTP/1.1 upstream on the HTTPS domain. Select a separate native TLS listener on 9090 behind the same service's TCP proxy for the next proof, preserving existing browser/MCP HTTPS. Verify server hostname and CA; do not disable TLS checks. The HTTPS-authenticated enrollment response will bind the trusted control endpoint/CA to avoid another manual grant. This candidate is not declared working until the real native client proves calls, trailers, bidirectional traffic and reconnect.

The TCP candidate is now verified: deployment `99905d94-08c1-427c-8bd3-9dc8bf0cf5cd`, native client exit 0, verified CA/hostname, unary/status trailers, independent bidirectional traffic and reconnect cursor. Adopt this two-listener topology; durable replay and real collaborator credentials remain separate implementation gates. The synthetic probe is disabled after proof.

### Model context lifecycle

Architect sessions are keyed by decision/question ID and architect runner ID. New decisions start fresh sessions with task/spec/commit, question, factual evidence and relevant accepted decisions; retain the exact session through all rounds. Parallel decisions have independent sessions. Persist final votes/rationale/decisions before releasing working context. Executor sessions are keyed by assignment/attempt: a new package starts fresh with its contract and repository revision; questions, council answers and review continuation keep that exact session. Persist terminal artifacts, commits and checks before releasing it. Releasing active context does not delete durable provenance or audit records. An interruption is not permission to reset the task context or the council round cap.

### Client defaults and inference preflight

Omitted model/effort preserves each client's own defaults. Explicit CLI values take precedence over private runner configuration and require a current provider/model capability check before any work session starts. Discover catalog/effort support through verified client metadata or authenticated read-only discovery; do not infer account availability from syntax acceptance or substitute a hardcoded generic catalog. Invalid values fail startup with current model/effort choices, without fallback. Catalog discovery failure is an explicit configuration error. Resolve/persist effective settings after successful native startup so exact-task resume remains consistent, while new tasks follow the runner's current defaults.
