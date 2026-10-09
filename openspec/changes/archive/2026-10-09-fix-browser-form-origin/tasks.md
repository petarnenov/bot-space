# Tasks

## 1. Browser mutation origin evidence

- [x] 1.1 Update browser mutation protection so explicit `Origin` or `Referer` headers remain authoritative and `Sec-Fetch-Site: same-origin` is accepted only when both are absent. Verify focused identity tests cover same-origin, cross-site, and contradictory headers.
- [x] 1.2 Separate sanitized responses for invalid origin, malformed form data, and invalid CSRF tokens. Verify existing CSRF and origin tests continue to reject forged mutations without changing application state.

## 2. Production regression verification

- [x] 2.1 Run the focused identity and PostgreSQL HTTP intake tests. Verify that a valid objective submission succeeds when the browser suppresses the referrer and that an invalid CSRF token still returns `403 Forbidden`.
- [ ] 2.2 Run the complete repository verification suite with a disposable `TEST_DATABASE_URL`, then run strict OpenSpec validation for `fix-browser-form-origin`.

## Requirement-to-check traceability

| Requirement | Verification tasks |
| --- | --- |
| Logout and browser mutation protection | 1.1, 1.2, 2.1, 2.2 |

## Workflow follow-up

- Apply this change only after an explicit apply request.
- Archive the change only after every task is implemented and verified.

## Archive warning

The owner explicitly requested archive on 2026-10-09 with task 2.2 incomplete. The focused regression checks and strict OpenSpec validation passed, while the unrelated runner race test `TestTwoIndependentSupervisorsDelegateAndContinue/claude_to_copilot` timed out twice.
