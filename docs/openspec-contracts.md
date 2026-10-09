# Versioned OpenSpec contracts

`internal/contracts.Content` binds project/root UUIDs, exact human input revision,
immutable GitHub repository ID, exact Git base commit, OpenSpec change/tasks,
acceptance scenarios and SHA-256 artifact digests. Proposal, design, tasks and
at least one change-scoped specification are mandatory. Moving branch names do
not substitute for exact commits. Paths cannot escape the selected change.

Canonical normalisation sorts task/scenario references and copies artifact
maps before hashing bounded JSON. Equivalent ordering retains the same hash;
changing bound content creates a new contract identity. `Matches` rejects
changed human revisions, artifact digests or a different approved hash.

These validation primitives do not grant execution authority. Durable publishing,
authenticated architect ownership and majority-backed contract approval are
still being implemented. Actual Git artifact reads/OpenSpec validation evidence
must be bound before assignment; supplied digest strings alone do not prove
files exist or checks passed. No production contract API is claimed.

Run `go test -race ./internal/contracts` for canonical ordering, stale human/spec
rejection, required artifacts, scope/path confinement and exact base commits.

## Durable architect publication

Store.Publish authenticates an actual machine token, requires the architect
role and matching project, rechecks activation/credential epoch in the write
transaction, and locks the active root/project mapping. A changed current human
revision or repository mismatch rejects publication. Canonical content/hash,
publisher and immutable root-revision reference persist in migration 0008.
Idempotent content retry retains the same contract/publisher and one safe audit
record. Exact historical contracts survive later human input revisions.

Store.Get is restricted to authenticated project architects; executor assignment
context will be delivered through the allocation service. Publication is not
approval and cannot authorize execution. Majority-backed approval and factual
Git/OpenSpec validation evidence remain mandatory later gates. No approval bit
is accepted from a publishing client.

Real-PG tests verify executor denial, durable replay/restart, changed input
rejection, unchanged historical provenance and one publication audit. Migration
0008 has not been applied to production yet. Contract delivery/API and actual
artifact-evidence gathering remain under implementation.

## Committed artifact verification

Content.VerifyGit checks an exact commit in an explicitly configured absolute
checkout and reads artifact blobs from that commit, not dirty working files.
It rejects missing commits, missing paths, symlink/non-file Git modes, oversized
blobs and mismatched SHA-256. Git is invoked with argument arrays and a bounded
technical context; repository/object replacement environment overrides are not
inherited. Verification does not execute repository code.

Tests use an isolated real Git repository to prove dirty files do not replace
committed evidence and false commit/digest claims fail. OpenSpec CLI semantic
validation and scenario/task-reference extraction remain pending; these Git
checks alone do not prove a complete valid specification or council acceptance.
