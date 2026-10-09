# Spec Governed Work

## Purpose

Execute human objectives through versioned OpenSpec contracts and architect-authorized evidence, integration and archival.

## ADDED Requirements

### Requirement: Versioned OpenSpec work contracts
Each runnable package SHALL identify its human root, OpenSpec change/task, approved artifact hashes, repository/base commit and acceptance scenarios. Architects SHALL produce and validate proposal/spec/design/tasks before implementation. Missing or stale contracts SHALL block assignment, not trigger human approval.

#### Scenario: Changed specification
- **GIVEN** a package approved for one specification hash
- **WHEN** the specification changes
- **THEN** the old approval cannot authorize the revised package.

### Requirement: Project-confined execution and artifacts
Executors SHALL use configured local project mappings and isolated working branches or worktrees. Incoming instructions SHALL not expand local permissions. Results SHALL include bounded artifact/commit references and scenario evidence. File sharing between physical machines SHALL not be presumed.

#### Scenario: Repository mismatch or forbidden path
- **GIVEN** an executor with a configured checkout
- **WHEN** assigned work references a different base revision or forbidden path
- **THEN** it reports the mismatch and the council decides a permitted resolution.

### Requirement: Autonomous verification and integration
Architects SHALL decide review, retry, acceptance, merge and deployment within configured project authority by the council policy. Required tests and named scenarios SHALL have factual evidence before acceptance and OpenSpec synchronization/archive. Failed checks or absent permissions SHALL block the corresponding action without a human override.

#### Scenario: Majority cannot waive missing evidence
- **GIVEN** an integration proposal with failing required checks
- **WHEN** architects support it
- **THEN** server policy prevents acceptance until required evidence is satisfied.

### Requirement: One change branch with separate review worktrees
Each OpenSpec change SHALL use its own branch derived from main and an isolated executor worktree. Architects SHALL fetch published executor commits and analyze that branch in their own review worktrees. Review evidence and council votes SHALL bind the exact commit and contract hash. New executor commits SHALL invalidate previous head approval. Merge into main SHALL require successful implementation checks and majority acceptance of the reviewed head, with revalidation against current main.

#### Scenario: Architect reviews executor branch
- **GIVEN** an executor publishing a commit on its OpenSpec branch
- **WHEN** architects inspect the implementation
- **THEN** each fetches and checks out that exact commit in a separate review worktree without changing the executor worktree or evaluating an unrelated main checkout.

#### Scenario: Branch advances after review
- **GIVEN** accepted review evidence for one commit
- **WHEN** the executor pushes another commit or main changes before integration
- **THEN** stale head approval cannot authorize merge, and required review or integration checks run against the updated revisions.

### Requirement: Publish every completed implementation task
After each completed small OpenSpec task, the executor SHALL commit its changes and push to the change branch, then notify architects with task ID, exact commit and factual check results. Architects SHALL be able to fetch that commit immediately after successful publication. A local commit or failed push SHALL not count as published review evidence. Retries SHALL preserve commit identity and avoid duplicate task completion.

#### Scenario: Incremental publication
- **GIVEN** an executor completing one task within an unfinished OpenSpec change
- **WHEN** its commit is successfully pushed
- **THEN** architects receive the task/commit/check references and can fetch and analyze those changes before the remaining tasks finish.

#### Scenario: Push fails
- **GIVEN** a local task commit and unavailable remote
- **WHEN** publication fails
- **THEN** the commit is retained with publication-pending status and no false claim of architect-visible completion.

### Requirement: Validated successful publication
Each implementation-task commit SHALL pass relevant checks and required regression gates before publication. Completion SHALL require confirmed successful push and architect notification. Failed checks or divergent remote state SHALL trigger executor reconciliation before retry. Network failure SHALL retain publication-pending status until push succeeds; history SHALL not be overwritten to fabricate success.

#### Scenario: Checks or publication fail
- **GIVEN** a task with failing checks, a remote conflict or a failed push
- **WHEN** the executor attempts completion
- **THEN** it repairs or retries the failed step and cannot report published completion until checks and remote publication are confirmed.
