# Tasks

## 1. Sign-in output

- [x] 1.1 Add terminal-aware sign-in output and wire it into the shared enrollment callback; verify exact hyperlink target/text, sequence closure, plain redirected output, `TERM=dumb`, and write-error handling with `go test ./cmd/runner`.
- [x] 1.2 Add local enrollment regression coverage for architect and executor with `--no-open`; verify both print the full verified URL on its own line and preserve the existing URL-verification boundary with `go test ./cmd/runner ./internal/runneridentity`.
- [x] 1.3 Update README runner startup instructions to describe Cmd+click and the plain-text fallback; verify documented flags match the existing Make targets and CLI.

## 2. Integration verification

- [x] 2.1 Run `go test -race ./cmd/runner ./internal/runneridentity`, build the runner for macOS and Linux, run `git diff --check`, and validate this change with `openspec validate clickable-runner-sign-in --strict`; record results and any unverified terminal interaction in this change's verification notes.

## Requirement-to-check traceability

- Clickable browser sign-in output / Architect sign-in: 1.1, 1.2 and 2.1; exact OSC 8 target and text are automated checks. Actual Cmd+click remains a manual check in a supporting terminal and must not be reported as passed without observing it.
- Redirected sign-in output and Dumb terminal: 1.1 and 2.1; regression checks require no hyperlink control sequences.
- Executor sign-in: 1.2 and 2.1; both roles exercise the shared callback.

## Workflow follow-up

- Archive only after implementation and recorded verification are complete.
