# Human Work Queue

## Purpose

Maintain a shared durable project backlog whose business objectives originate from authenticated humans and are executed autonomously by architects.

## ADDED Requirements

### Requirement: Human-originated root intentions
Only an authenticated current human member with project access SHALL create a root intention from a story, ticket reference or description. Runner credentials SHALL not authenticate the intake API. Root records SHALL retain immutable creator, project, original description, priority and revision provenance.

#### Scenario: Human intake versus runner impersonation
- **GIVEN** an authorized human and a registered runner
- **WHEN** each submits a root objective
- **THEN** human intake persists its provenance and the runner cannot impersonate a human creator.

### Requirement: One authoritative project backlog
One PostgreSQL-backed queue SHALL represent each project within its workspace. Architect views SHALL refer to that same queue. Duplicate intake keys SHALL return one identical root; changed payload SHALL conflict. Cross-workspace references SHALL be rejected and queued work SHALL survive restart.

#### Scenario: Shared queue and restart
- **GIVEN** several architects viewing one project
- **WHEN** a human submits an objective and the server restarts
- **THEN** all authorized views refer to the same retained root without duplicate execution.

### Requirement: Derived scope and version changes
Architect-created work SHALL trace to a human root and versioned scope. Derived tasks SHALL not create independent business objectives or grant new local permissions. Changed root input SHALL create a new revision requiring an architect decision; it SHALL not silently replace an assigned contract.

#### Scenario: Root changes during execution
- **GIVEN** an executor assigned a particular root/spec revision
- **WHEN** new human input or an architect scope revision arrives
- **THEN** existing contracts remain identifiable and changed work requires a new recorded council decision.

### Requirement: Human outside the operational loop
Planning, scope interpretation, assignments, clarifications, retries, review, acceptance and authorized integration/deployment SHALL be decided autonomously by architects. No per-task human approval or human override SHALL be required. Missing authority or unresolved decisions SHALL produce blocked states rather than fabricated permission.

#### Scenario: Autonomous work after intake
- **GIVEN** a human-created root and configured project authority
- **WHEN** architects process it through planning and execution
- **THEN** the workflow progresses without a human approval gate.
