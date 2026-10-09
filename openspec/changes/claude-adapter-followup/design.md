# Design

## Context

This follow-up reintroduces Claude-specific provider work that was intentionally
removed from `distributed-agent-runner` to unblock Codex/Copilot completion. The
goal is to add Claude support/evidence without regressing the existing runner
contracts and without weakening privacy, ownership, or bounded retry behavior.

## Goals / Non-goals

### Goals

- Provide verified Claude Code start and exact resume support through the runner
  provider interface.
- Prove Claude participates in cross-provider continuation matrices with Codex
  and Copilot via independent supervisors.
- Keep current secret/input/output bounds and normalized failure surface.

### Non-goals

- Reworking task schema or mailbox authorization.
- Changing Codex/Copilot adapter behavior beyond compatibility fixes required by
  Claude parity.

## Provider Integration

- Add/update Claude adapter implementation under the existing provider interface.
- Validate supported Claude CLI version and authentication preconditions at
  startup; return explicit actionable startup errors when missing.
- Keep bounded stream parsing and output truncation semantics consistent with
  current provider adapters.

## Evidence Plan

- Synthetic tests: provider-level parser/permission/error handling coverage.
- Real execution tests: at least one Claude task execution and exact resume
  continuation through runner-managed sessions.
- Cross-provider tests: Codex→Claude, Claude→Copilot and Copilot→Claude
  continuity over two turns with normalized result/error/session metadata.

## Documentation

- Update runner and client docs to reflect three-provider support only after
  evidence exists.
- Update any user-facing matrix tables/samples to include Claude-specific
  commands, expected output, and failure modes.
