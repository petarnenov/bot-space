# Architect council engine

`internal/council` implements pure decision rules. It has no clock, timeout,
network connection, human override or automatic offline-member removal.

Create a decision with `New(id, kind, members, material, proposal)`. Supported
kinds are plan, allocation, answer, review, retry and integration. The service
must authenticate members and validate immutable material references against
project records before invoking this engine. Required approval is always
`floor(member_count / 2) + 1`.

`Cast(member, round, proposalHash, choice)` accepts approve, object or
need_information. Identical retries are idempotent; conflicting second votes,
outsiders and stale hashes/rounds are rejected. Each proposal remains immutable
within its round. Acceptance occurs immediately at strict majority. If every
member responds without majority, `Revise` opens the next round with fresh
votes. Three unsuccessful rounds produce `blocked_no_majority`. Missing votes
remain pending indefinitely.

`Reconsider` creates a linked new decision only after material changes to
trusted source/spec/evidence/council revisions or operational proposal actions.
Changing only rationale, decision ID or member ordering cannot reset the cap.
Removing members requires an explicit council revision validated by the server;
disconnection alone is insufficient. Material identifiers are not arbitrary
client restart labels. The persistent service must also prevent reopening a
previously exhausted identical material decision through a new `New` call.

`Snapshot` returns defensive copies for views and persistence. The engine is
intentionally single-owner: the service must serialize mutations using its
transaction/lock and persist vote evidence atomically. This package alone does
not implement GitHub authorization, durable storage or distributed coordination.

Verify rules with `go test -race ./internal/council`. Tests cover thresholds,
ties, missing votes, exhaustion, reconsideration, duplicate/stale/outsider votes
and immutable snapshots. Durable coordinator races are a separate tracked task.

`Restore(snapshot, material)` reconstructs a persisted decision by replaying every
round and vote through the same engine. It verifies material/proposal hashes,
fixed membership, the strict-majority threshold and all resulting statuses.
Forged acceptance, duplicate vote evidence, unsupported round advancement and
changed material fail closed. A restart retains three-round exhaustion rather
than resetting it. Database authorization, coordinator leases and transactional
storage still belong to task 5.1; this reconstruction does not provide them.

## Durable planning decisions (in progress)

Migration 0011 introduces decision snapshots, durable heads, immutable rounds
and votes, accepted proposal commits, and renewable coordinator leases. It has
only been applied in isolated test databases, not production.

`internal/councilstore.OpenPlan` resolves a validated contract and current root
revision/lifecycle epoch, snapshots all active registered architects (including
those with expired credentials), verifies their GitHub access, and creates one
planning head per root under a transaction/advisory lock. Plan actions target the
exact contract. Repeated creation returns the existing decision; changing a label
cannot create a second head or reset blocked rounds.

`Vote` reauthenticates the current project architect, locks the decision, checks
the current root fence, replays stored evidence, and atomically persists the vote,
snapshot, majority commit and safe audit. Conflicting, stale, outsider or revoked
votes fail. Rounds/votes/commits reject UPDATE and DELETE. On reload, normalized
round/vote/commit records must match the snapshot: a valid-looking snapshot without
actual durable votes fails closed. `Get` preserves access to historical decisions
after their root becomes fenced; reading history grants no execution authority.

`Coordinate` grants a 30-second renewable lease for proposal writing only.
Expired leases can be taken over with a higher fencing epoch; `Revise` requires
the exact current owner/epoch, round and proposal hash. This technical lease is
not a discussion or task duration limit. Missing votes remain pending regardless
of coordinator expiry. Three fully responded unsuccessful rounds remain blocked
across restart. `ReconsiderPlan` links materially changed proposals or verified
source/membership revisions to the exhausted history. New authoritative root/spec
input can replace an earlier in-flight or accepted plan with a linked decision;
client labels and membership changes alone cannot replace such a plan.

Real-PG race tests cover concurrent creation/voting, one acceptance/audit, fixed
offline membership, coordinator contention/takeover, stale lease rejection, all
three tied rounds, cosmetic restart rejection, material/source reconsideration,
revoked access, pause fences, retained history and forged snapshot rejection.
Storage tests seed trusted contract-validation fixtures; actual committed Git and
OpenSpec verification is covered separately by the contract integration test.

Task 5.1 remains open: answer/review/retry/integration decisions still
need authoritative work/question/evidence subject bindings, and native control
handlers/runtime are not yet wired. The storage foundation does not assign work,
resume a coding session or merge a branch. Outbox replay and autonomous model
coordination remain tasks 5.2 and 5.3.

Council mutations reverify the exact GitHub actor/repository mapping before the
transaction and bind that mapping again under row locks. A repository mapping
change between verification and the write fails closed; a regression test races
that change. Contract consumption additionally requires the contract repository
ID to match the current project mapping.


## Task-scoped allocation decisions

Migration 0012 extends durable heads with a server-derived subject. Existing
plans retain subject `root`; an allocation uses `task:OPEN_SPEC_TASK_ID` from
its verified contract. Previous-decision links and heads have composite foreign
keys tying project, root, kind and subject together. Migration 0012 is currently
verified only in isolated test databases.

`OpenAllocation` and `ReconsiderAllocation` accept one `assign` action whose
`Target` is an active executor registered for the project and whose `Value` is
the exact task ID. The server rechecks GitHub project access for the executor.
Contract content must match its canonical hash and stored provenance, and the
task must exist in its verified task list. Independent tasks have independent
heads; a missing task, mismatched action or foreign-project executor fails.
Approvals recheck executor eligibility. Objections remain recordable if an
executor loses access so the council can complete its round and revise.

`LockAccepted` is the transaction gate for trusted control-service consumers.
It verifies exact decision kind, subject and contract hash, normalized majority
evidence, the current head, and current root/spec/lifecycle fences. Allocation
consumption additionally requires the current plan to have a proven majority
for that same contract and lifecycle. A decision
for task 1.1 cannot authorize task 1.2 or a review action. Reading old decisions
preserves history. Superseded heads reject votes, coordinator acquisition,
proposal revisions and operational consumption even if their old contracts
still match the human input revision.

The upcoming assignment transaction must also authenticate its actor, validate
current executor capability/online authority and reserve its global machine slot.
An accepted allocation proposal currently reserves no capacity and starts no
provider. Attempt/question/evidence bindings, retries and integration decisions
remain pending in tasks 5–6 and 8; task 5.1 remains unchecked.

Real-PG race tests verify independent task heads, exact task/action matching,
missing-task and foreign-executor rejection, withdrawn-executor objections versus
approvals, majority persistence, wrong-subject/kind/hash denial, unaccepted
consumption denial, pause fences and old-head rejection after a new verified
commit with the same human revision.
