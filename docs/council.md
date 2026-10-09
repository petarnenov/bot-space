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

Task 5.1 remains open: allocation/answer/review/retry/integration decisions still
need authoritative work/question/evidence subject bindings, and native control
handlers/runtime are not yet wired. The storage foundation does not assign work,
resume a coding session or merge a branch. Outbox replay and autonomous model
coordination remain tasks 5.2 and 5.3.

Council mutations reverify the exact GitHub actor/repository mapping before the
transaction and bind that mapping again under row locks. A repository mapping
change between verification and the write fails closed; a regression test races
that change. Contract consumption additionally requires the contract repository
ID to match the current project mapping.
