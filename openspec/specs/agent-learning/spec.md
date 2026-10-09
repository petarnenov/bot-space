# agent-learning Specification

## Purpose
Help architects and executors avoid repeated mistakes through durable evidence-backed lessons supplied to fresh work sessions.

## Requirements

### Requirement: Evidence-backed lesson candidates
Failures and verified corrections SHALL create idempotent lesson candidates identifying source project/task/decision, exact spec/commit, structured failure signature, cause, failed approach, verified resolution and prevention check. Candidates SHALL retain artifact/check references and exclude credentials. Unreviewed model statements SHALL not become authoritative lessons.

#### Scenario: Failed check with a verified correction
- **GIVEN** an executor or architect encountering a failure and a later verified correction
- **WHEN** learning capture runs
- **THEN** one versioned candidate links the failure and correction evidence without duplicating the same event.

### Requirement: Autonomous majority validation
Architects SHALL accept, reject or revise a lesson through the existing fixed-council majority policy without human approval. Promotion SHALL bind the exact candidate revision and evidence. Missing correction evidence SHALL block promotion even with majority support. General cross-project lessons SHALL be explicitly reviewed for applicability and data isolation.

#### Scenario: Unsupported or stale promotion
- **GIVEN** a lesson lacking verified correction evidence or changed since voting
- **WHEN** promotion is requested
- **THEN** it remains non-authoritative and cannot seed another agent's session.

### Requirement: Scoped retrieval for fresh sessions
Every new architect decision session and executor task session SHALL receive a bounded set of accepted, current, applicable lessons matched to authorized project, task/failure kind and version constraints. The session SHALL record the lesson revision IDs supplied. Unrelated project details, rejected or revoked lessons SHALL not enter its context. Fresh-session isolation SHALL remain intact.

#### Scenario: Clean session with relevant memory
- **GIVEN** a new task and accepted lessons for relevant and unrelated projects
- **WHEN** the executor starts a fresh session
- **THEN** it receives only authorized applicable lesson revisions with prevention checks, without inheriting previous conversations.

### Requirement: Recurrence and versioned correction
Repeated failure signatures SHALL record recurrence against supplied lesson revisions and trigger architect review of why prevention failed. A retry SHALL not blindly repeat a known failed approach; it SHALL bind an accepted revised mitigation. Contradicting evidence SHALL allow architects to revise or revoke a lesson while preserving history. Raw lesson content SHALL not enter operational logs.

#### Scenario: Repeated known mistake
- **GIVEN** a task seeded with a lesson whose failure signature recurs
- **WHEN** the failure is reported
- **THEN** recurrence evidence is recorded and architects decide an updated mitigation or lesson revision before retry.
