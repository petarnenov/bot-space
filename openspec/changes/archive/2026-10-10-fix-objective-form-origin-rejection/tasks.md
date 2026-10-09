# Tasks

## 1. Establish the failure

- [x] 1.1 Add bounded origin/CSRF rejection categories to identity middleware diagnostics; test representative origin/CSRF categories and prove response bodies do not echo submitted data.
- [ ] 1.2 Reproduce the rendered Objective form with native Chromium and WebKit POSTs over local HTTPS. Record browser version, effective referrer policy, origin classification and response. Do not synthesize browser security headers in the success path.

## 2. Correct form origin evidence

- [ ] 2.1 Use the production rejection category and deployed origin configuration to identify and correct the actual cause without weakening origin or CSRF checks. Keep open until authenticated production evidence is available.
- [ ] 2.2 Verify valid browser submission reaches the Objective detail page and persists one root, revision and audit record. Test absent metadata compatibility, null/foreign/malformed/duplicate Origin rejection, invalid CSRF, revoked project access, and idempotent retry with database state assertions.

## 3. Verification and rollout evidence

- [x] 3.1 Run focused identity, browser and PostgreSQL intake tests, `make verify`, and strict OpenSpec validation; record outcomes accurately in a focused verification document.
- [ ] 3.2 Verify the deployed revision and public-origin configuration, reload the production page, and complete one owner-approved browser submission. Record its nonsecret ID, response status and successful detail page; keep this task open if live access or approved test content is unavailable.

## Requirement-to-check traceability

| Requirement | Checks |
| --- | --- |
| Safe browser mutation rejection diagnostics | 1.1, 1.2, 3.1 |

## Workflow follow-up

- Apply after an explicit apply request.
- Synchronize specs and archive after verification. Never equate planning completion or a passing CLI test with resolution of the production form incident.
