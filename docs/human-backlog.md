# Human backlog foundations

The project backlog stores human-originated intentions independently of runner
credentials and legacy inbox memberships. The human intake implementation includes immutable revisions, authenticated
create/read/list, idempotency and browser forms.
Revision append/read, bounded listing and CSRF-protected browser intake are implemented.

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

Migration 0007 is additive and has been deployed after isolated database tests.
Routes are enabled with runner identity; production verification is recorded below. Run real PostgreSQL tests with
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

## Creator lifecycle controls

Migration 0010 adds lifecycle epochs, archived status, reconciliation fencing,
and append-only lifecycle history. It is not deployed yet. The creator with
current GitHub access can POST the CSRF-protected `/control` route with `action`
and `expected_epoch`; another collaborator or agent bearer cannot act for them.
Concurrent/stale actions conflict instead of silently overriding current state.

Pause changes the root to `paused`. Resume changes it to `blocked` with
`reconciliation_required=true`; execution cannot restart until an authorized
council decision reconciles interrupted effects. Cancel is terminal. Archive is
allowed only for paused, cancelled or completed roots and removes them from active
lists while their URLs, input revisions, provenance and lifecycle history remain
readable. Archived and terminal roots reject input revisions. Lifecycle history
rejects UPDATE and DELETE at the database layer. The detail page shows actions
available for the current state, the execution fence and lifecycle history.

`backlog.LockExecutable` checks the exact input revision and lifecycle epoch,
active state, archive status and reconciliation fence under a shared transaction
lock. Derived writes must hold this lock together with their own current
runner/council authority. Human actions take an exclusive root lock. Publication,
validation and allocator eligibility reject paused, terminal, archived or
unreconciled roots. Durable assignment/event delivery and council reconciliation
must consume this fence in tasks 5–6 before task 4.3 is marked complete; provider
termination and uncertain-side-effect recovery are not claimed by these controls.

Real-PG lifecycle and HTTP tests cover creator access, agent/CSRF rejection,
concurrent pauses, restart, cancelled-root revival denial, archive visibility,
retained immutable history and contract-gate rejection after pause/resume.
Run `go test -race ./tests/integration -run 'TestHuman|TestContractPublication'`
with `TEST_DATABASE_URL` and the documented OpenSpec toolchain.

## Production verification (2026-10-09)

Hosted CI 37930781666 passed PostgreSQL, race, build, vulnerability and OpenSpec
gates. Deployment e72cacfb-fdde-4758-9713-546ea2e61514 reached SUCCESS and applied
migration 0007. The authenticated GitHub browser opened the configured project's
backlog with title, description, ticket reference, priority and submit control,
without an additional invitation. Production verification did not create a
fictional business objective. Actual submit/revision/restart/concurrent-key and
cross-project/workspace behavior is covered by real-PG HTTP/integration tests.
Health/readiness remained 200, GitHub login 302, unauthenticated MCP 401.
Creator lifecycle controls remain a separate task; no autonomous execution is
claimed from intake alone.
