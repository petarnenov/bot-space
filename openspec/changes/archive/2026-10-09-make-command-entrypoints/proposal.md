# Proposal

## Why

Repository operations were being run through direct tool commands. The owner
requested a single project entrypoint through `make`, and this must be tracked
through OpenSpec protocol instead of ad-hoc edits.

## What Changes

- Add a root `Makefile` with reusable targets for compose lifecycle, core
  verification, OpenSpec commands, and Railway checks/deploy.
- Update `README.md` command examples to use `make` targets as the standard
  entrypoint.
- Keep behavior unchanged: targets wrap existing commands and arguments.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- None (tooling/docs workflow only; no spec-level behavior change).

## Impact

Improves command consistency and repeatability without changing runtime
semantics of mailbox, runner, or task orchestration features.
