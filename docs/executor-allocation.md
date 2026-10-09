# Executor allocation foundations

`internal/workallocation` reserves one machine slot keyed by verified GitHub
actor and executor public key. Project-scoped credentials for the same key
share that slot. Migration 0013 has only been applied in isolated test databases.

`Presence` authenticates the executor, force-verifies project access and binds
its exact repository/identity/credential mapping under row locks. It records a
bounded client description and a 90-second renewable technical liveness lease.
Omitted model/effort stay omitted. An available heartbeat cannot release a
reservation or change an occupied machine's client configuration. Real provider
catalog/settings verification remains a runner responsibility in task 7.

`Assign` requires a current authorized architect, an accepted allocation for the
exact task/spec/lifecycle and an accepted current plan for that same contract.
The executor must be active, currently authorized and online with a matching
credential epoch and available presence. Decision and global machine locks
serialize creation and capacity checks. Assignment and slot generation commit
atomically with one safe audit. Identical retries return the existing assignment.

A composite foreign key binds the assignment to its allocation decision's
project/root/task subject. A partial unique index also prevents concurrent
unresolved assignments for the same logical root/task. Capacity stays occupied
in question wait, review or uncertain interruption. Presence expiry never frees
it; completion/reconciliation release must be backed by the later review flow.
`Busy` exposes only the authenticated machine's occupancy bit, including work
in another project, without returning that other project's task contents.

There is no task deadline, elapsed execution budget or automatic expiry release.
Presence and credential bounds protect connectivity/authority only. Native
control handlers, local scheduling/session execution, renewable task authority,
question/evidence transitions and majority-backed release remain pending.
Tasks 5.1 and 6.1 are not marked complete from these foundations.

Real-PG race tests reserve one machine concurrently through two projects and
verify one winner, an occupied rejection, retry idempotency, shared busy state,
provider-change denial, retained capacity during question wait/interruption and
presence expiry. Contract-validation fixtures are seeded for storage tests;
actual Git/OpenSpec validation is independently verified by the contract test.
Run `go test -race ./tests/integration -run TestDurableCouncil` with
`TEST_DATABASE_URL` and the pinned repository toolchain.

## Durable attempts and session binding

Migration 0014 adds numbered attempts with a saved native session and a renewable
90-second authority lease. It is currently tested only in isolated databases.
`Begin` requires the assigned project executor, its exact client configuration,
current majority plan/allocation, live presence and the same reserved machine
slot/generation. Repeating Begin returns the same first attempt. An interrupted
or expired attempt cannot be replaced with a new context through this method.

The runner captures the actual native client session and calls `BindSession`.
A different session is rejected, including by a database trigger. `Renew` checks
the saved session, authority epoch, current root/spec/plan, slot and presence.
It renews starting, running and question-wait authority. Starting can renew before
the native session has been reported, so initialization has no total time budget.
Renewal after authority loss fails; capacity remains occupied for reconciliation.

Tests verify idempotent Begin, cross-project denial, client/session replacement
denial, immutable session storage, same-session renewals while waiting, and no
fresh context after expiry. Old assignment/attempt creation timestamps demonstrate
that the database gate imposes no legacy execution-age cap. This is not a claim
of actual two-day native provider execution. Native adapters, local persistence,
lease-loss observation, majority-backed retry/reconciliation and session-bound
questions/results still need integration before tasks 6.1–6.3 are complete.
