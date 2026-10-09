# Human backlog foundations

The project backlog stores human-originated intentions independently of runner
credentials and legacy inbox memberships. This implementation is in progress;
immutable first revisions, authenticated create and idempotency are implemented.
Revision editing, listing and browser intake remain pending in OpenSpec 4.1.

`backlog.Store.Create` authenticates the opaque browser session through the
existing identity store, verifies current GitHub project access and rechecks the
session inside the write transaction. An agent bearer cannot supply a human
identity. The project mapping determines workspace/repository scope; no client
workspace ID or creator ID is accepted.

Inputs include project ID, intake key, title, description, optional ticket
reference and priority 0–4. Titles are limited to 256 bytes, descriptions to
32 KiB and ticket references to 2 KiB. Text must be valid UTF-8 without NUL.
Keys are printable ASCII up to 128 bytes. Titles/descriptions trim boundary
whitespace before fingerprinting; text content is not rewritten.

A project/creator/key identifies one root. Identical retries return its original
revision; changed payload conflicts. The original creator, project and first
input are retained. A root and its revision/audit commit atomically, with a
composite foreign key preventing cross-project/workspace records. Deferred
current-revision integrity prevents roots without source input. Audit metadata
contains references and revision, not task bodies or credentials.

Migration 0007 is additive and has only been applied in isolated test databases.
No production human intake route is claimed. Run real PostgreSQL tests with
TEST_DATABASE_URL and `go test -race ./tests/integration -run TestHumanBacklog`.
Tests cover restart/idempotency, changed keys, machine impersonation, revoked
sessions, removed project access and one authoritative root/audit record.
