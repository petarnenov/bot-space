# Machine Runner Delta

## MODIFIED Requirements

### Requirement: Three provider adapters
Runner-managed sessions SHALL support Codex CLI, Claude Code and GitHub Copilot
CLI through installed supported versions and machine-local authentication.
Startup SHALL report missing binaries, unsupported protocol features and
authentication requirements explicitly. Adapters SHALL normalize final text,
session identity, failure and interruption without claiming an untested
provider works.

#### Scenario: Provider compatibility
- **GIVEN** each configured supported provider
- **WHEN** an agent executes and continues a task
- **THEN** its result and exact persistent session ID are captured through that
  provider's supported interface.
