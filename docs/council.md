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
