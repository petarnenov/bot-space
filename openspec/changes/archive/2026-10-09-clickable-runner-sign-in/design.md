# Design

## Context

See proposal.md for motivation. `make architect` starts the shared runner with
`--no-open`. Its enrollment callback in `cmd/runner/main.go` currently prints
`Open GitHub sign-in:` followed by a plain URL on the same line. The identity
client validates the URL against the configured server before invoking that
callback. The CLI targets macOS and Linux.

## Goals / Non-Goals

**Goals:** Keep the verified URL intact and clickable, including wrapped URLs,
while keeping redirected output readable.

**Non-Goals:** Detect every emulator's hyperlink support or change terminal
settings. The visible full URL remains available for copying.

## Decisions

Use OSC 8 hyperlink output with the complete verified URL as target and visible
text. Close the hyperlink before the following newline. This addresses wrapping
without relying only on heuristic URL detection. Print the prompt separately.
Plain URLs alone were considered but do not explicitly identify wrapped links.
Reference: [terminal hyperlink protocol](https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda).

Limit hyperlink output to terminal file descriptors with `TERM` other than
`dumb`; other writers use plain text. Use the existing platform dependencies
for terminal detection, with platform-specific ioctl constants if necessary,
rather than adding a dependency. Preserve the existing verified-URL callback
and propagate output errors through its existing sanitized error.

Use the shared callback for both roles. Formatting tests exercise exact target,
visible URL, sequence closure, plain output, and write errors. Integration tests
exercise terminal selection and enrollment callback wiring without browser
launch or production access.

## Risks / Trade-offs

- Emulator support varies → keep the full URL visible for automatic detection
  and copying; manually verify Cmd+click in the user's terminal before claiming
  that interaction passed.
- Escape sequences could leak into logs → gate them on terminal output and
  cover redirected output and `TERM=dumb`.
- URL encoding could change the authentication destination → preserve the
  already verified URL byte for byte and test its full query string.

## Migration Plan

Rebuild the runner through the existing Make targets. No state or server
migration is required. Rollback restores the previous output formatter.
