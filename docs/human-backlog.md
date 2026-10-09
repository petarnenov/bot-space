# Human backlog foundations

The project backlog stores human-originated intentions independently of runner
credentials and legacy inbox memberships. This implementation is in progress;
immutable first revisions, authenticated create and idempotency are implemented.
Revision append/read are implemented; bounded listing and CSRF-protected browser intake are implemented; human lifecycle controls remain pending in OpenSpec 4.1.

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
Routes are wired behind runner identity activation; migration 0007 and intake deployment are pending production verification. Run real PostgreSQL tests with
TEST_DATABASE_URL and `go test -race ./tests/integration -run TestHumanBacklog`.
Tests cover restart/idempotency, changed keys, machine impersonation, revoked
sessions, removed project access and one authoritative root/audit record.

The original human creator can append a revision using an expected current
revision. The transaction locks the root and current human session, revalidates
the project mapping and writes revision/current-pointer/audit atomically. Stale
expected revisions conflict. Get can retrieve an exact historical revision or
the latest input. Revisions do not overwrite the original source or silently
change existing execution contracts; council/contract invalidation wiring is
tracked in subsequent tasks. Tests verify unchanged original provenance and
stale-write rejection.

Browser routes are /projects/PROJECT_UUID/intentions and individual intention
pages. Forms submit human input or append a new input revision; they do not
approve plans, retries, merge or council decisions. Browser sessions and
same-origin CSRF are required; an agent bearer without a browser session is
rejected. HTML templates escape external content. Lists use project-scoped
creation-order cursor UUIDs, 50 records per page, and show current input revisions.
Tests verify human submit, bearer/CSRF rejection and script-text escaping.

The user also approved creator-owned pause/resume/cancel and non-destructive
archive with retained history. Those controls are tracked separately in task
4.3 and are not yet exposed by these intake pages.
