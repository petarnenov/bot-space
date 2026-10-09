## ADDED Requirements

### Requirement: Safe browser mutation rejection diagnostics
Browser mutation rejections SHALL return a bounded reason code distinguishing origin mismatch, opaque origin, malformed or multiple origins, Fetch Metadata rejection, malformed form and CSRF failure. The code SHALL NOT expose cookies, tokens, request bodies, raw Referer URLs or user-supplied header values. Public error bodies SHALL remain sanitized.

#### Scenario: Diagnose rejected browser mutations
- **GIVEN** a browser mutation rejected by an origin, form parsing or CSRF check
- **WHEN** an operator inspects its response diagnostics
- **THEN** a fixed reason code identifies the rejecting gate without revealing session credentials, request headers or form contents.
