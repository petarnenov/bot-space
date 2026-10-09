# Proposal

## Why

Valid production form submissions can be rejected with HTTP 403 when a browser suppresses both `Origin` and `Referer` while the application itself sets `Referrer-Policy: no-referrer`. Browser mutation protection must recognize trustworthy same-origin Fetch Metadata without weakening CSRF validation.

## What Changes

- Accept `Sec-Fetch-Site: same-origin` as same-origin evidence only when `Origin` and `Referer` are absent.
- Keep explicit foreign `Origin` or `Referer` authoritative and rejected.
- Keep the authenticated session and session-bound CSRF token mandatory.
- Return distinct sanitized errors for origin, form parsing, and expired CSRF failures.
- Add regression coverage for missing-referrer browser submissions and cross-site rejection.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `human-authentication`: Define valid same-origin browser context when referrer data is intentionally suppressed.

## Impact

The change affects browser mutation middleware in `internal/identity`, its tests, and the human authentication specification. It does not change OAuth, session storage, authorization, machine APIs, or mutation endpoint semantics.
