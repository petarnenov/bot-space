# Proposal

## Why

When starting an architect with `make architect`, the printed browser sign-in
URL should open with Cmd+click in a supporting terminal. A long plain-text URL
can wrap and become difficult for terminal URL detection to recognize.

## What Changes

- Render the verified GitHub sign-in URL as an explicit terminal hyperlink in
  interactive output, using the full URL as both its target and visible text.
- Print the URL on its own line and preserve plain text when output is redirected
  or the terminal is declared dumb.
- Cover the shared architect/executor enrollment output with regression tests.
- Scope is output formatting only. Enrollment, URL verification, browser launch
  defaults, credentials, and server APIs remain as implemented. No later change
  is needed for this request; terminal-specific configuration is outside scope.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `runner-identity`: Clickable verified browser sign-in output during enrollment.

## Impact

`cmd/runner/main.go`, nearby CLI tests, and the runner startup instructions in
`README.md`. Existing Make targets use this shared output. No new dependency or
server/API change is planned.
