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
