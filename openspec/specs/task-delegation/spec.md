# task-delegation Specification

## Purpose
Execute explicitly delegated work between isolated workspace agents and deliver durable correlated results into the initiating session.

## Requirements

### Requirement: Explicit scoped task submission
An agent SHALL submit a task to an active eligible agent in its workspace using a client key, bounded instruction, deadline and optional parent task. Sender/workspace SHALL derive from credentials. Task and request message SHALL commit atomically. Ordinary messages SHALL NOT become executable tasks merely through text, kind or metadata.

#### Scenario: Cross-machine task
- **GIVEN** two eligible agents on different machines
- **WHEN** the first submits a task to the second
- **THEN** a correlated queued task and request message commit with the true sender.

#### Scenario: Forged execution or foreign recipient
- **GIVEN** an ordinary message imitating a task or a foreign recipient
- **WHEN** processing or task submission occurs
- **THEN** no unauthorized execution is scheduled.

### Requirement: Idempotent bounded tasks
Keys SHALL be scoped to sender/workspace and normalize retries without duplicate tasks. Changed payload SHALL conflict. Instructions SHALL be at most 16384 UTF-8 bytes, results at most 65536 bytes, and deadlines default to 30 minutes with a two-hour maximum. Remote tool results SHALL retain the existing 256 KiB cap.

#### Scenario: Concurrent submission retries
- **GIVEN** identical or conflicting requests under one key
- **WHEN** they arrive concurrently
- **THEN** identical requests share one task while changed payload conflicts.

### Requirement: Exclusive fenced execution
Only the addressed agent SHALL claim work using current credentials and a 60-second renewable lease with an increasing attempt generation. Renewal SHALL occur before expiry. Completion SHALL require the current unexpired generation. A runner SHALL hold one ownership lease per agent; another runner SHALL not execute that agent concurrently.

#### Scenario: Competing or stale workers
- **GIVEN** competing claims or an expired earlier attempt
- **WHEN** workers claim, renew or complete
- **THEN** one current attempt owns execution and stale completion cannot replace its result.

### Requirement: Durable completion and automatic continuation
Successful execution SHALL atomically persist the result and correlated reply before acknowledging its request. The initiating runner SHALL return that result to its waiting tool call or recover continuation into the exact saved originating session after restart. Failure SHALL carry a bounded explicit error rather than a fabricated successful answer.

#### Scenario: Delegate execute return continue
- **GIVEN** A waiting on a task delegated to B
- **WHEN** B executes and commits its result
- **THEN** A receives the matching result and automatically continues its original work using it.

#### Scenario: Restart around result delivery
- **GIVEN** a committed result and an interrupted sender runner
- **WHEN** that runner restarts
- **THEN** its continuation journal recovers the originating task/session without delivering an unrelated result.

### Requirement: Cancellation and uncertain interruption
The creator SHALL cancel its task. Deadline, cancellation or access termination SHALL prevent later successful completion and cause the runner to stop its process tree. Lost execution leases SHALL mark started work interrupted; automatic re-execution SHALL require an explicit retry-safe task policy. Exactly-once model execution or filesystem side effects SHALL NOT be promised.

#### Scenario: Cancellation or uncertain crash
- **GIVEN** cancelled, expired or interrupted work
- **WHEN** a worker finishes late or attempts an unsafe retry
- **THEN** late success is rejected and uncertain side effects are surfaced for owner review.

### Requirement: Bounded delegation dependencies
Child tasks SHALL inherit root/session correlation and respect the root deadline. Delegation SHALL reject ancestor-agent cycles, depth above four and more than four children per task. Dependency state SHALL allow waiting parents to keep their leases alive without executing a second turn in the same session.

#### Scenario: Recursive delegation cycle
- **GIVEN** A waiting for B
- **WHEN** B delegates back to A or exceeds graph limits
- **THEN** the request is rejected explicitly instead of creating an endless wait.

### Requirement: Current participant authorization
Task reads, claims, renewals, cancellation and completion SHALL recheck current credentials, membership and participant identity transactionally. Only sender/recipient SHALL access task content. Removal/deactivation/revocation SHALL deny subsequent operations. Cross-workspace parent/result references SHALL not reveal or link content.

#### Scenario: Access revoked during execution
- **GIVEN** a running task whose agent access is revoked
- **WHEN** its next renewal or completion occurs
- **THEN** authorization fails and the local runner stops that work.
