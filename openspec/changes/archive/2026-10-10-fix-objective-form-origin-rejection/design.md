# Design

## Context

`identity.Web.Protect` authenticates, checks `sameOrigin`, parses the form, then verifies CSRF. The reported message identifies the second step; presence of a posted token proves neither its validity nor the request origin.

Current `sameOrigin` compares an explicit Origin literally with `Config.BaseURL`, otherwise validates Referer, otherwise accepts absent or same-origin Fetch Metadata. Thus the previous missing-header fix does not address an explicit `Origin: null`, a configuration mismatch, or a different Fetch Metadata value. These are hypotheses, not observed production headers. No authenticated failing request or deployed build identifier was captured during this proposal.

Some authenticated renderers set `Referrer-Policy: no-referrer`. Existing regression tests synthesize headers rather than exercise browser-generated requests. Native browser probing against a local HTTPS form fixture showed a same-origin Origin and `Sec-Fetch-Site: same-origin` on POST even with that policy, so changing referrer policy alone is not a supported correction. `make objective` uses a privileged database path and cannot validate this flow.

## Goals / Non-Goals

Goals: identify the actual rejecting branch, restore normal browser submissions, and prove durable creation through the UI. Preserve session, CSRF and project checks.

Non-goals: blanket acceptance of null origins, trusting proxy headers as browser evidence, changing operator CLI intake, OAuth, queue execution, or unrelated runner tests.

## Decisions

1. Add typed rejection reasons returned internally by the origin check and exposed through sanitized diagnostics. Use a finite set of categories only; never log raw headers, body fields, cookies or tokens. Test both status and absence of sensitive data.
2. Reproduce native form submissions from the rendered project page in Chromium and WebKit against local HTTPS. Do not set request Origin or Fetch Metadata manually in the browser test. Capture only header classifications, browser version, effective policy and response status in durable evidence.
3. Use bounded rejection categories to identify the production gate. Correct only the observed request/configuration mismatch; do not weaken explicit null, foreign, malformed or multiple Origin rejection, accept untrusted Host/forwarding headers, or change referrer policy without evidence.
4. Confirm the configured public origin and deployed source revision match the browser URL. Correct deployment configuration if evidence identifies a mismatch. Keep the live verification task open when authenticated request headers or deployment state are unavailable.
5. Success requires a complete GET-form-POST-redirect flow and exactly one Objective, initial revision and audit record. Bad CSRF, null/foreign origin and revoked access must create none. Retrying with the same idempotency key must not duplicate records.

## Risks / Trade-offs

- The failing headers are not yet known → require a recorded reproduction before claiming root cause; policy changes alone are not evidence of resolution.
- Local browser tests cannot establish the actual production request → require bounded production rejection classification before selecting a correction.
- Local tests cannot establish deployed behavior → track a separate live browser check and deployment revision. A successful CLI insert or CI run is insufficient.

## Migration Plan

No schema changes. Run focused regressions and repository verification, deploy the tested revision within the owner's authorization, reload the production form and verify one owner-approved Objective submission. Do not reuse credentials pasted in chat. Record only nonsecret evidence. Roll back the code/configuration change if authorization or privacy regressions appear. An unavailable live environment leaves the live task open and the incident unverified.
