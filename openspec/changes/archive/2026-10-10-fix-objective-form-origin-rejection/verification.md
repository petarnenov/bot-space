# Verification

## Completed locally

- `go test ./internal/identity ./internal/web ./internal/taskweb` passed.
- The PostgreSQL-backed `TestHumanIntakeHTTPRejectsAgentsAndCSRFAndEscapesContent` passed with `TEST_DATABASE_URL` set to the local PostgreSQL 16 container. It verifies invalid CSRF and explicit foreign, opaque, malformed and duplicate Origin classifications; an ordinary successful request with no synthesized browser headers; redirect and detail rendering; exactly one Objective, initial revision and creation audit event; and no duplicates after retrying the same idempotency key.
- `make verify` passed: format check, module verification, vet, full tests, race tests, build, vulnerability scan, and strict validation of all OpenSpec specs and changes.
- A native form POST probe using Playwright 1.63 with Chromium and WebKit against a local HTTPS fixture completed successfully with `Referrer-Policy: same-origin`, producing a same-origin Origin and Fetch Metadata and following the redirect. This fixture was not the application, did not use application authentication, and does not identify the production rejection branch.

## Still unverified

- The authenticated production request's `Origin`, `Referer`, `Sec-Fetch-Site`, bounded `X-Request-Rejection` code and deployed source revision have not been captured. Railway CLI access was unavailable in this workspace.
- No correction to the production rejection cause can be selected safely until that evidence is available. The implementation adds bounded rejection categories; it does not claim to fix or resolve the reported production incident.
- A real browser submission through the deployed application and verification of the resulting production Objective remain outstanding.
