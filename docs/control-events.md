# Durable control events

Migration 0017 stores per-recipient issued/acknowledged counters and immutable
protobuf outbox records. It is tested only in isolated databases, not deployed.

`controlevents.PublishTx` is a trusted service operation used inside the domain
transaction. Assignment creation and majority-answer acceptance now publish their
executor messages atomically with their records. A scoped event key deduplicates
identical retries; changed payload conflicts. Per-recipient row locks serialize
sequence allocation and transaction commits. A rollback leaves no event or gap.
Offline registered runners retain their events; transmission grants no authority.

`Page` force-verifies current runner/project access and binds the repository and
credential mapping under locks. It starts after the durable server ACK, returns
at most 32 events and 192 KiB, with payloads limited to 128 KiB each. Reading or
sending does not advance acknowledgement. `Acknowledge` requires the exact next
sequence and event UUID; skip-ahead acknowledgements fail. Repeated old/exact
acknowledgements are harmless. Another scoped identity cannot access the journal.

On Darwin/Linux, `State.PersistDelivery` validates contiguous sequences, retains
payloads and fingerprints, and returns an ACK only after private file/directory
fsync. Journals use the state directory's no-follow/ownership/0600 protections.
Restart restores unapplied events. Changed replay payloads, cursor gaps and reused
IDs fail. At 16 unapplied events the client refuses another ACK until processing
frees space. Applied payloads compact while up to 128 replay fingerprints remain.
`MarkDeliveryApplied` must follow a durable domain transition, not receipt alone.

Real-PG/race tests cover rollback, concurrent ordering, idempotent publication,
changed keys, replay after send/restart, exact/skip/duplicate ACKs, immutable history
and revoked access. Client tests cover private restart recovery, replay conflicts,
applied-event compaction and backpressure/closed-journal failure without ACK.
Domain tests verify assignment and exact-session answer messages commit together.

## Native integration and verification

The mailbox service installs `orchestration.Backend` for authenticated self
inspection, durable ACKs, architect votes and event streaming. The control
handshake validates resume against the durable server ACK; zero requests replay
from that position. A client cannot skip unacknowledged data. Pull tracks sent
positions, checks activation/epoch while idle, refreshes project authorization
periodically and force-verifies access before delivering data.

Council changes now publish to fixed members; human lifecycle changes publish
root ID/epoch/action/state to registered project runners. Assignments and answers
retain their atomic publication. The protobuf adds a RootControl message.

`runner serve` subscribes on each scoped connection. It uses zero on reconnect,
persists before ACK, deduplicates stored replay and leaves pending messages for
the role dispatcher. Network/credential renewal replaces connections without
clearing the journal. A full inbox refuses ACKs and bounds memory/storage.
Receipt alone starts no coding process. Domain handlers in later runtime tasks
must apply durable messages idempotently and mark them applied afterward.

Real gRPC/PG tests reject a skip-ahead resume, disconnect after local persistence
before ACK, create new server/backend and local-state objects, replay the pending
event and verify exact durable ACKs and one pending copy. Tests additionally cover
council and root-control publication, wrong ACK UUID/order, revoked access and
backpressure without ACK. Native provider execution/resume remains tasks 5.3,
6 and 7; deployment remains task 10. Delivery completion does not claim those
runtime behaviors or a production rollout of migrations 0008–0017.


## Resume checkpoint

The user requested stopping after task 5.2 and resuming only on `continue`.
Task 5.2 is implemented and verified against its full tracked delivery scope.
Code checkpoint: `ca00f0c` on `openspec/architect-led-orchestration` in
`/Users/petarnenov/bot-space-orchestration`. The task/documentation commit follows
that checkpoint. OpenSpec progress is 9/30 complete, 21 remaining.

Verification on a clean committed worktree passed `go vet ./...`,
`go test -race ./...` with real PostgreSQL, `go build ./...`, and all 15 OpenSpec
validation items. Native gRPC tests cover disconnect before ACK and restart of
both backend/server objects and private local state; delivery and domain-state
fixtures remain distinct from actual provider execution. AGENTS.md remains
byte-identical to `da7285d`.

Resume by inspecting the actual branch/status and running pinned OpenSpec apply
instructions. Earlier open tasks 4.3 and 5.1 retain their partial foundations:
creator lifecycle controls, plan/allocation/answer storage, one global executor
slot, attempts and exact-session questions/answers. They still need operational
reconciliation, review/retry/integration subjects and complete runtime wiring.
Next work should finish those dependencies and task 5.3 autonomous architect
sessions; continue groups 6–10, including five provider adapters, physical Copilot
on 192.168.1.223 via VPN, worktree integration, UI, learning and final delivery.
Do not reset completed delivery work or mark those pending tasks done.

Production still has migrations through 0007 and the earlier backlog deployment.
Migrations 0008–0017 and the new operational backend have only been verified in
isolated test databases/local native tests. Production deployment and end-to-end
native provider proof remain task 10. Keep GitHub OAuth credentials distinct from
GitHub App metadata-verification credentials; never print private variables.

Historical superseded prototype changes remain in the original shared workspace
and as unstaged/untracked files in the orchestration worktree: `.env.example`,
`compose.yaml`, old config/mailbox/MCP task additions, `internal/runner/`,
`internal/tasks/`, old distributed-runner plan, historical reports/research and
legacy task tests. They were present before this delivery task and are preserved;
review them before reuse rather than publishing their obsolete deadline/human
approval behavior or private notes. All delivery changes are committed separately.
