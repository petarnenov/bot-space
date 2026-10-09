# Tasks

## 1. Operator queue intake

- [x] 1.1 Add a backlog operator-intake method that resolves an existing GitHub user, verifies current project authority, and reuses objective validation, idempotency, revision, and audit semantics; verify PostgreSQL tests cover success, unknown user, denied authority, retry, and conflicting payload.
- [x] 1.2 Add `mailbox objective` argument parsing for project, GitHub user ID, title, exactly one description source, priority, ticket, and key; verify focused CLI tests cover valid input, missing input, mutually exclusive descriptions, invalid priority, and sanitized failures.

## 2. Make interface and documentation

- [x] 2.1 Add `make objective` with validated variables and safe argument passing to the mailbox CLI; verify inline and file-based descriptions create the expected Objective through a disposable PostgreSQL database.
- [x] 2.2 Document prerequisites, examples, defaults, idempotent retries, and failure behavior in README.md and Make help; verify every documented command matches the implemented interface.

## 3. Integration verification

- [x] 3.1 Run focused CLI/backlog integration tests, `make verify`, and strict OpenSpec validation.

## Requirement-to-check traceability

| Requirement | Verification tasks |
| --- | --- |
| Human-originated root intentions | 1.1, 1.2, 2.1, 2.2, 3.1 |

## Workflow follow-up

- Apply only after an explicit apply request.
- Archive only after implementation and verification are complete.
