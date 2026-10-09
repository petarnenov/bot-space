# Tasks

## 1. Browser mutation validation

- [x] 1.1 Change origin validation so missing `Origin` and `Referer` do not reject an authenticated mutation before CSRF validation; verify focused identity tests cover missing metadata, explicit same-origin evidence, and explicit foreign evidence.
- [x] 1.2 Extend the PostgreSQL HTTP intake regression test to submit a valid originless objective and reject invalid CSRF and explicit foreign-origin submissions without state changes.

## 2. Verification

- [x] 2.1 Run focused identity and PostgreSQL HTTP intake tests and verify the objective POST redirects successfully for valid originless input.
- [ ] 2.2 Run the repository verification gates and strict OpenSpec validation.

## Requirement-to-check traceability

| Requirement | Verification tasks |
| --- | --- |
| Logout and browser mutation protection | 1.1, 1.2, 2.1, 2.2 |

## Workflow follow-up

- Apply only after an explicit apply request.
- Archive only after implementation and verification are complete.

## Archive warning

The owner explicitly requested archive on 2026-10-09 with task 2.2 incomplete. Focused regression tests and strict OpenSpec validation passed, while the unrelated runner race test `TestTwoIndependentSupervisorsDelegateAndContinue/codex_to_copilot` timed out.
