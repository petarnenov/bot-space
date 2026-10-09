# Runner enrollment foundations

The startup flow binds a machine Ed25519 key, project and role to a verified
GitHub identity. This implementation is in progress: signing primitives, durable enrollment, one-use challenge consumption and
credential issuance/refresh are implemented; startup CLI and production server configuration integration remain pending under OpenSpec task 3.2. Production identity enrollment is verified below; full serve execution remains unfinished.

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
Migrations 0005/0006 were deployed during identity preparation; execution remains disabled.

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

Credential issuance serializes claims by machine key/role and rejects a
conflicting GitHub actor in another project. Real-PG tests verify one actor can
hold two independent project scopes while the same key cannot change actor
between repositories. This identity invariant does not replace the pending
machine-wide single-task reservation enforcement.

## Startup session integration

NewSession binds the HTTP client to an open server/role state journal. Acquire
per project refreshes existing key-bound identity or enrolls a new scope, then
persists the returned lease before exposing it to the supervisor. It does not
silently replace revoked, corrupt or foreign-scoped state. Refresh also journals
its new epoch. Real-PG/mock-GitHub HTTP tests verify enrollment, refresh and
credential persistence together. Full production startup command wiring remains
pending.

## CLI authentication entrypoint and server switch

`runner enroll --server https://SERVER --state /private/executor --role executor
--project PROJECT_UUID` acquires and journals identity; repeat `--project` for
additional eligible scopes. It outputs only runner/project/role/epoch metadata,
never the lease or bearer token. This authentication entrypoint does not yet
implement the final automatic `serve` execution/control runtime.

Enable HTTP enrollment with RUNNER_IDENTITY_ENABLED=true only after configuring
GITHUB_APP_CLIENT_ID, GITHUB_APP_INSTALLATION_ID, GITHUB_APP_PRIVATE_KEY,
CONTROL_ENDPOINT, CONTROL_TLS_CERT and CONTROL_TLS_KEY.
The App needs Metadata read and generates its verification token automatically.
App private keys and browser OAuth client secrets are separate credentials. Certificate hostname must match the native endpoint. The switch
is off by default, preserving the existing browser/mailbox deployment. Configured
projects must exist in PostgreSQL; project setup and live activation remain
pending. Do not interpret enabled HTTP identity as a ready execution backend.

## Open-stream revocation evidence

The real-PG/mock-GitHub integration also opens a native gRPC TLS stream using an
issued machine token and a hostname/CA-verified client. After PostgreSQL runner
deactivation, the next frame on that same connection returns Unauthenticated
and never reaches the fixture backend. This confirms per-frame persisted
activation checking, not durable event storage; the fixture emits a synthetic
receipt. Production native control wiring remains a separate unfinished gate.

DialNative builds a native gRPC connection from the HTTPS-issued lease, checks
CA/hostname through TLS and attaches the bearer only through transport-secure
per-RPC credentials. Calls enforce 256-KiB message bounds. The real-PG open-stream
revocation test now exercises this actual client helper. Refresh requires a new
connection with the new epoch and retained durable cursor; the runtime replay
loop remains pending.

## Operator project registration

After migrations and App configuration, run:

```sh
mailbox bootstrap-project --workspace bot-space --repository petarnenov/bot-space
```

The command resolves immutable repository/owner IDs using the App and requires
successful collaborator-endpoint access before storing a project. It emits only
project/repository ID metadata. Repeated bootstrap retains the same project ID;
unknown workspaces, inactive mappings or changed owner/name fail closed. This
is platform setup and creates no memberships, invitations or individual agent
grants. Other configured repositories receive their own project IDs/scopes.

Real-PG tests verify stable bootstrap and owner-change rejection. Live production bootstrap is verified below.

## Production project bootstrap evidence (2026-10-09)

Deployment `7b327639-0e65-4728-ac31-cc4ca86bd7e1` applied the six bundled
migrations. Railway rejected a two-command pre-deploy setting, so a separate
single-command bootstrap ran after migration completion on deployment
`a7ffa586-514a-467c-9987-01da095925fb` (SUCCESS). Safe logs confirmed project
`bb25680f-eeea-4cde-b229-ddec09961c73` for GitHub repository `1410902803`.
The native service pre-deploy setting was restored to `/mailbox migrate`.

Health/readiness returned 200, GitHub login 302 and unauthenticated MCP 401.
Runner identity/execution remain disabled pending authenticated native service
and automatic serve integration; project bootstrap alone is not task completion.

## Live automatic identity flow (2026-10-09)

Deployment `9043e66b-b3d4-49f1-942d-0350c2eac02a` reached SUCCESS with GitHub
App-backed HTTP enrollment and native TLS identity enabled. The real CLI
`enroll --no-open` produced a verified same-server login URL; the existing
GitHub browser session completed OAuth automatically. No invitation or manual
server role grant occurred. Architect runner
`71a80d7f-e92d-495f-83c1-7d114bc8e217` received epoch 1 for project
`bb25680f-eeea-4cde-b229-ddec09961c73`.

`runner doctor` then refreshed to epoch 2 and verified authenticated native TLS
self-inspection through `thomas.proxy.rlwy.net:39004`, exiting 0. CLI output
contained only runner/project/role/epoch; bearer and machine key stayed private.
`--no-open` supports headless hosts without requiring desktop browser launch.
Task/presence operations remain closed with FailedPrecondition until the durable
work backend is installed. Automatic `serve` runtime is still pending, so
OpenSpec task 3.2 is not checked off yet.

## Identity lifecycle owner

Lifecycle acquires configured scopes at startup and refreshes three minutes
before credential expiry. Refresh/new-connection notification succeeds before
a scope is locally usable. Any failed refresh invalidates that scope immediately
because a lost response could already have rotated the server token. Other
project scopes remain independently valid. Key-proved retry uses bounded
backoff, and a restored scope requires a newer epoch for the same runner/role.
These authentication bounds never create an execution deadline.

The supervisor must replace native connections on OnLease and fence affected
work on OnLoss; durable replay/execution reconciliation remain separate tracked
runtime gates. Tests verify scope isolation, invalidation, backoff and recovery.

## Automatic serve identity owner

`runner serve` now automatically acquires configured project identity at startup,
verifies native TLS self-inspection, retains the process/state lock and renews
credentials through Lifecycle. Each successful refresh replaces the corresponding
project connection; loss closes only that scope. Shutdown and partial startup
failure close all acquired connections. Console status is metadata-only.

A real production serve using the architect state reached identity_ready at
epoch 3 and remained running until SIGINT, then exited cleanly. Unit/race tests
verify automatic multi-scope acquisition, safe output and connection cleanup.
This completes the identity startup component; task dispatch, model sessions and
durable event replay remain separate unimplemented runtime tasks. identity_ready
must not be presented as available execution capacity or completed work.
