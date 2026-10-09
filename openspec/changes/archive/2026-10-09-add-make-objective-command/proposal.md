# Proposal

## Why

Operators need a reproducible terminal command to add an Objective directly to the project queue without opening the browser UI. The command must preserve the same human provenance, project authorization, validation, idempotency, and audit history as web intake.

## What Changes

- Add `make objective` with explicit project, GitHub user, title, description, priority, ticket, and optional idempotency inputs.
- Add a mailbox CLI command that creates the Objective through the existing PostgreSQL queue model.
- Require an existing human identity and current repository authority for the supplied project instead of allowing anonymous or runner-authenticated intake.
- Print the created Objective ID and project ID on success and return a nonzero exit code with sanitized errors on failure.
- Document inline and file-based descriptions so long assignments do not depend on shell quoting.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `human-work-queue`: Permit trusted operator CLI intake on behalf of an existing authorized human while preserving queue provenance and invariants.

## Impact

The change affects `Makefile`, the mailbox CLI, the backlog store, focused CLI/integration tests, and README command documentation. It introduces no schema, dependency, HTTP API, or UI changes.
