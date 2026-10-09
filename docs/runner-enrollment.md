# Runner enrollment foundations

The startup flow binds a machine Ed25519 key, project and role to a verified
GitHub identity. This implementation is in progress: signing primitives, durable enrollment, one-use challenge consumption and
credential issuance/refresh are implemented; startup CLI and production server configuration integration remain pending under OpenSpec task 3.2. No production enrollment endpoint is claimed.

## Implemented boundaries

`runneridentity.StartMessage` uses a versioned domain-separated message binding
project UUID, architect/executor role and a random 32-byte nonce. `VerifyStart`
rejects role, project or nonce substitution. Claim/refresh signatures use a
separate message domain, resource UUID and operation; they cannot be reused
between resources or purposes. Helpers validate signatures; the eventual store
must atomically consume expiring challenges to prevent replay.

`Store.Begin` persists only key-proved enrollment for an active configured
project. Identical unexpired requests return the same enrollment after restart;
expired requests require a fresh nonce. Begin never grants a credential.

`Store.AuthorizeGitHub` is reserved for a validated OAuth callback, not a normal
GET or browser session read. It forces current repository owner/collaborator
verification and binds the resulting immutable GitHub ID. Another person cannot
replace that binding, and a repository mapping changed during verification
cannot inherit the old admission result. This does not require legacy mailbox
membership or a The Firm invitation.

## Persistence and verification

Migration 0006 adds project mappings, enrollments, role-bound runner records and
one-use challenge storage. Migration 0005 preserves the previously verified
legacy task foundation unchanged; its dormant tables are not the new autonomous
work runtime. New orchestration execution will have no task duration limit.
Neither migration has been deployed to production at this stage.

Run `go test -race ./internal/runneridentity` for cryptographic boundaries.
With a configured TEST_DATABASE_URL, run
`go test -race ./tests/integration -run TestRunnerEnrollment` for real PostgreSQL
restart/idempotency, wrong-role signatures, unauthorized identity, expired
attempts and repository changes during admission. Claim/refresh tests additionally cover single-use and concurrent replay, wrong
key proof, expired challenges, hashed token storage, epoch rotation, collaborator
removal, runner deactivation and safe transactional credential audit. The full
startup OAuth/HTTP/native stream integration still must pass before task 3.2
can be completed.

## Credential lifecycle

Store.Issue requires a valid key proof and current forced GitHub verification.
It atomically consumes the challenge, rotates the hashed credential and writes
safe audit metadata. Challenges expire after one minute; credential lifetime is
15 minutes. These are authentication bounds, not task execution deadlines;
the runtime must refresh while arbitrarily long work continues. Authentication
checks active project/runner, current epoch, role, owner and GitHub authority
before accepting each operation. A lost issuance response requires a new
challenge; reusing the old signature cannot deliver another credential.

## HTTP and OAuth callback

Runneridentity.Web registers strict bounded JSON machine endpoints for begin,
challenge and credential issuance. Signed machine routes reject browser cookies
and Origin headers and have per-peer/global admission limits. Pending challenge
storage is bounded per resource and removes expired challenges. The ordinary
success page is informational and cannot authorize enrollment.

The identity callback invokes AfterGitHubLogin only after state-cookie/PKCE
validation, one-use OAuth attempt consumption and provider identity verification.
The runner hook forces repository admission for the enrollment's immutable
project/role/key. A machine-only credential response includes the authenticated
control endpoint and public CA, preserving TLS hostname/chain validation.
Real-PG/fake-provider HTTP tests prove a collaborator without mailbox membership
can enroll, while replay and cross-origin requests are denied. Production wiring
and real runner startup remain unfinished; task 3.2 is not complete.

## Startup client library

Runneridentity.NewClient accepts HTTPS origins or explicit loopback HTTP for
local testing. Enroll signs the project/role request, validates the returned
same-server login URL, invokes the caller's browser opener and polls bounded
machine challenges until GitHub authorization succeeds. The machine HTTP client
has no browser cookie jar and never follows redirects. Credential/trust metadata
is checked before returning a lease. Refresh proves the same key and requires a
new credential epoch for the same runner/project/role.

Real PostgreSQL/mock-GitHub integration verifies this client enrolls an architect
and refreshes its credential. Unit tests reject unsafe origins, external login
URLs and machine API redirects. The private key/credential journal and production
startup CLI wiring remain pending; these tests do not claim a deployed CLI login.

## Private machine state (Linux and macOS)

OpenState requires an absolute owner-private directory, fixes its server/role binding and holds an exclusive flock for the lifetime of the runner. A
second process cannot open the same state, while architect/executor directories
on one host remain independent. The Ed25519 seed is durable across restarts.
Files require mode 0600, the directory must exclude group/other access, and
owner identity is checked. Directory-descriptor-relative operations and
O_NOFOLLOW prevent reading or writing through state-file symlinks.

Credential updates use an owner-private temporary file, fsync, atomic rename
and directory fsync. Separate per-project lease files bind project/role and may be loaded after
credential expiry for key-proved refresh; loading an expired lease does not
make it valid for gRPC authentication. The state object does not expose secrets
through JSON marshaling. Startup must additionally keep state outside managed
project worktrees and confine model access to approved execution resources.

Race tests cover stable identity, exclusive locks, two roles on one host,
credential persistence/expiry and unsafe permissions/symlinks. Production CLI
startup and native-provider filesystem confinement remain separate unfinished
gates.

State tests also executed successfully on physical Linux machine 192.168.1.223
through VPN using a cross-compiled test binary. This verifies Linux state/lock
behavior, not authenticated Copilot task execution. Temporary test files were
removed after the run.

Startup does not require the shell's current directory to be the project root.
Use absolute configuration, state and repository mappings. The agent's working
directory is the assigned task worktree, while private state remains outside
project source. Changing shell working directory must not switch identity or
select a different checkout implicitly.

## Multi-project identity correction

The machine key/process lock is shared for one role across its eligible
configured projects. OpenState no longer accepts or fixes a project. SaveLease
and LoadLease(projectID) use independent scoped files, so credentials cannot
replace another project's lease. Version-1 project-bound profiles migrate to
version 2 while preserving the key and any saved credential under its project
scope. Human intent/contract chooses the target repository; allocation still
must enforce one active executor task globally across all project connections.
That allocator gate is not implemented by the journal alone.
