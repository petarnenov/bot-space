# Runner enrollment foundations

The startup flow binds a machine Ed25519 key, project and role to a verified
GitHub identity. This implementation is in progress: signing primitives and
durable enrollment are implemented; browser callback wiring, one-use challenge
consumption, credential issuance/refresh and startup CLI integration remain
pending under OpenSpec task 3.2. No production enrollment endpoint is claimed.

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
attempts and repository changes during admission. SQL challenge storage alone
does not prove replay protection; claim/refresh tests must pass before task 3.2
can be completed.
