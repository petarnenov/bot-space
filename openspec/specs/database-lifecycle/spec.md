# database-lifecycle Specification

## Purpose

Preserve durable service state in PostgreSQL and provide safe, repeatable schema upgrades across restarts and deployments.

## Requirements

### Requirement: Durable PostgreSQL state
All durable application state SHALL reside in PostgreSQL. Application restart or replacement SHALL preserve committed records when the same database is used. Application filesystem contents SHALL NOT be required to recover durable state.

#### Scenario: Application container replacement
- **GIVEN** a committed database fixture and the configured PostgreSQL database
- **WHEN** the application container is replaced without copying its filesystem
- **THEN** the fixture remains available from the same database.

### Requirement: Explicit migration command
The application image SHALL provide `mailbox migrate` that applies pending versioned migrations and exits zero on success. Already-applied migrations SHALL be skipped. Serving HTTP SHALL NOT automatically migrate the database; an outdated schema SHALL fail readiness.

#### Scenario: Repeated migration
- **GIVEN** all bundled migrations have already been applied
- **WHEN** `mailbox migrate` runs again
- **THEN** it exits zero without repeating schema changes or losing data.

#### Scenario: Service without migration
- **GIVEN** an empty database
- **WHEN** HTTP serving starts without running migrations
- **THEN** readiness returns 503 and no schema changes are applied automatically.

### Requirement: Serialized and atomic migrations
Concurrent migration commands SHALL serialize database changes, apply each migration once, and record its version and checksum atomically with its SQL changes. Failed migrations SHALL roll back and exit nonzero. Lock acquisition and execution SHALL have a combined five-minute deadline.

#### Scenario: Two simultaneous migrators
- **GIVEN** an empty PostgreSQL database
- **WHEN** two migration commands run concurrently
- **THEN** each migration is committed once and both commands complete successfully within the deadline.

#### Scenario: Migration failure
- **GIVEN** a migration containing a failing SQL statement
- **WHEN** the migration command executes it
- **THEN** its changes and version record are rolled back, the command exits nonzero, and earlier committed migrations remain intact.

#### Scenario: Lock contention deadline
- **GIVEN** another connection holds the migration lock past the deadline
- **WHEN** a migrator waits for that lock
- **THEN** it exits nonzero with a sanitized timeout error within five minutes plus scheduling tolerance.

### Requirement: Migration integrity
Migration execution SHALL refuse an altered checksum for an already-applied migration and refuse a database containing a migration version unknown to the running binary. Readiness SHALL also reject those conditions. Errors SHALL identify the migration version without including SQL content or credentials.

#### Scenario: Applied migration edited
- **GIVEN** an applied migration whose bundled SQL checksum has changed
- **WHEN** migration or readiness validation runs
- **THEN** migration exits nonzero or readiness returns 503, without applying further changes.

#### Scenario: Older binary against newer schema
- **GIVEN** a database containing a version absent from the binary
- **WHEN** that binary runs migrations or readiness validation
- **THEN** it refuses migration or reports not ready.

### Requirement: Bounded database connections
The service SHALL use a bounded pool, defaulting to ten connections per process, with a two-second connection timeout and explicit deadlines for database operations. Invalid pool settings SHALL fail configuration validation. Database errors SHALL produce sanitized operational failures.

#### Scenario: Pool exhaustion
- **GIVEN** all configured connections are occupied
- **WHEN** readiness requests another connection
- **THEN** it reports 503 within its two-second deadline rather than waiting indefinitely.
