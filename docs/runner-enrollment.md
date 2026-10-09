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
