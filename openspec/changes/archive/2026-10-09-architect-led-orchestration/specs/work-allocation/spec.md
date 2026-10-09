# Work Allocation

## Purpose

Allocate approved project work to suitable executors with authoritative capacity reservations and fenced evidence-based execution.

## ADDED Requirements

### Requirement: Capability and availability registration
Executors SHALL register provider/version, projects, tools and policy with exactly one execution slot, enforced by server and runner. Waiting for council answers or unresolved interruption SHALL retain that slot; switching provider/project SHALL not add slots. Advertised availability SHALL not authorize work or imply completion. Missing heartbeat SHALL indicate unknown/offline rather than free capacity.

#### Scenario: Executor advertises capacity
- **GIVEN** an enrolled executor with declared capabilities
- **WHEN** architects inspect availability
- **THEN** they see its capabilities and current reservations without treating an offline machine as available.

### Requirement: Majority-backed atomic assignment
Only an architect SHALL assign a work package supported by an accepted matching council decision. Assignment SHALL atomically reserve an eligible executor slot and persist the spec/repository revision and attempt epoch. Concurrent assignments SHALL not exceed capacity or assign one active package twice.

#### Scenario: Concurrent assignment race
- **GIVEN** one available slot and competing architect requests
- **WHEN** assignments commit
- **THEN** only one reservation succeeds and the other cannot create duplicate active work.

### Requirement: No elapsed execution deadline
Work packages and provider sessions SHALL have no elapsed execution deadline, default duration cap or inherited parent time budget. Elapsed time alone SHALL not fail, expire, cancel or retry work. Waiting for council answers SHALL not consume a task time budget. Technical heartbeat, RPC and renewable lease bounds SHALL measure connectivity and authority only; a healthy authorized executor SHALL be able to renew throughout arbitrarily long execution.

#### Scenario: Heavy task exceeds legacy time limits
- **GIVEN** a healthy authorized executor running a heavy task
- **WHEN** execution exceeds thirty minutes, two hours or any former default duration
- **THEN** the assignment and exact provider session remain active without expiration, forced cancellation or automatic retry.

#### Scenario: Technical connection is interrupted
- **GIVEN** a long-running assignment whose connection or lease is lost
- **WHEN** technical liveness checks fail
- **THEN** the system records an interruption requiring architect reconciliation, without classifying the task as failed because it took too long.

### Requirement: Fenced execution and safe reconciliation
Execution SHALL use renewable leases and attempt fencing. Late results from cancelled, superseded or unauthorized attempts SHALL not replace authoritative output. Lease loss SHALL not imply that local side effects were undone. Architects SHALL decide uncertain retries through the council, without a human review gate.

#### Scenario: Interrupted executor returns late
- **GIVEN** a lost execution lease and an uncertain local result
- **WHEN** reconnection or a retry is considered
- **THEN** stale completion is rejected and architect reconciliation determines the next action.

### Requirement: Questions and accepted results
An executor SHALL preserve its task/session when asking the council and continue after a version-bound accepted answer. Returned evidence SHALL be reviewed by architects against the assigned acceptance criteria before completion. CLI exit alone SHALL not satisfy work-package acceptance.

#### Scenario: Clarification and evidence review
- **GIVEN** an assigned executor requiring clarification
- **WHEN** a majority answer arrives and execution later returns artifacts
- **THEN** its exact session continues and architects accept or reject the evidence autonomously.

### Requirement: Contextual executor escalation
An executor unable to complete work SHALL ask the architect council with task/contract/attempt/session identifiers, repository branch and commit, attempted approaches, relevant diff/artifact references, factual checks/errors and the precise requested decision. Credentials and secrets SHALL be excluded. The assignment SHALL retain its single slot and session while awaiting a majority answer, then resume in that context. The executor SHALL not invent success or request human operational approval.

#### Scenario: Executor cannot resolve a blocker
- **GIVEN** a task blocked by a failing check, ambiguity or missing capability
- **WHEN** the executor asks architects for help
- **THEN** the council receives the relevant attempted-work evidence and exact revision, decides autonomously, and the executor continues from its preserved session after an accepted answer.

### Requirement: Task-scoped executor context
An executor SHALL start a fresh model session for each new work package, seeded with its approved contract, spec, exact repository revision and relevant accepted decisions. Questions, council answers, review feedback and continuation within that task SHALL retain the exact session. Terminal artifacts/checks/commits SHALL be durably recorded before releasing working context; a new task SHALL not inherit an unrelated prior session.

#### Scenario: Task completion and clarification
- **GIVEN** an executor receiving a new task after its previous task terminates
- **WHEN** it starts work and later asks architects for clarification
- **THEN** the new task starts with a fresh seeded session and clarification resumes that same session without losing the attempted-work context.

### Requirement: Human-targeted project routing and global machine capacity
The project referenced by human input and the approved contract SHALL determine execution routing. Architects and executors SHALL operate across eligible configured projects rather than remain assigned to one fixed project. An executor SHALL retain exactly one active reservation across all project scopes and connections of its machine role. Project-specific registrations SHALL not multiply capacity.

#### Scenario: Cross-project assignment race
- **GIVEN** one executor available to two projects and concurrent majority-backed assignments
- **WHEN** both projects reserve that machine
- **THEN** one assignment succeeds globally and the other waits for capacity, including while the first awaits council answers.
