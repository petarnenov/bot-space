# Proposal

## Why

Objective submission still returns the origin-rejection 403 after two previous fixes. The supplied form fields do not reveal the request headers: the error identifies the origin gate, not an invalid CSRF token. We need browser-level reproduction and a verified correction rather than another unproven relaxation of that gate.

## What Changes

- Identify the failing header branch with bounded, sanitized reason codes and reproduce the rendered form submission in a real browser.
- Correct the observed request/configuration mismatch without weakening the origin or CSRF protections. Browser probes show that changing the referrer policy alone does not explain the rejection.
- Keep explicit foreign, opaque (`null`), malformed, and multiple Origin values rejected; retain session-bound CSRF verification and the existing missing-header fallback.
- Cover the complete page GET, generated hidden fields, POST, redirect and durable Objective creation flow with browser and PostgreSQL regression checks.
- Require verification of the deployed build and actual production form before declaring the incident resolved.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `human-authentication`: Define sanitized rejection diagnostics while preserving current origin and CSRF requirements.

## Impact

Identity middleware, identity/intake tests, and a focused operational verification record. No database migration or change to queue authorization, OAuth, referrer policy, or `make objective`. New CLI features and unrelated runner test fixes are out of scope.
