# Proposal

## Why

Production browsers can omit `Origin`, `Referer`, and Fetch Metadata from an authenticated HTML form POST. The current middleware rejects these valid submissions before evaluating the session-bound CSRF token, so users cannot submit objectives.

## What Changes

- Accept a browser mutation with no origin headers when its authenticated session and session-bound CSRF token are valid.
- Continue rejecting any explicitly foreign `Origin` or `Referer`, regardless of CSRF or Fetch Metadata.
- Keep malformed forms and invalid CSRF tokens rejected with distinct sanitized errors.
- Add HTTP regression coverage for originless valid submissions and forged submissions.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `human-authentication`: Allow originless browser mutations to rely on the existing session-bound CSRF proof while retaining explicit foreign-origin rejection.

## Impact

The change affects browser mutation middleware in `internal/identity`, its HTTP regression tests, and the human authentication specification. OAuth, session storage, authorization, machine APIs, and endpoint semantics remain unchanged.
