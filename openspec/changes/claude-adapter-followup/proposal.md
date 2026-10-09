# Proposal

## Why

`distributed-agent-runner` is now scoped to Codex and Copilot so the main runner
delivery can complete without Claude account quota blockers. Claude adapter and
Claude-inclusive cross-provider evidence still need an explicit tracked change.

## What Changes

- Add Claude Code start/exact-resume adapter evidence and bounded parsing checks
  as a dedicated follow-up stream.
- Add Claude-inclusive cross-provider continuity checks between Codex, Claude and
  Copilot through independent runner processes.
- Update runner/task/mailbox documentation to restore three-provider guidance
  only after real Claude evidence is captured.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `machine-runner`: expand provider support/evidence back to include Claude Code.
- `mcp-mailbox-tools`: restore documentation contract language that references
  inactive unmanaged Claude sessions alongside Codex/Copilot.

## Impact

Planning and implementation touch runner provider adapter code, provider smoke
tests, cross-provider process evidence, and related docs/spec mappings. No scope
change to task storage schema, mailbox tool contract, or browser ownership model.
