# Machine Runner

## Purpose

Supervise local provider-neutral agents on separate physical machines and preserve their task/session continuity without inbound public machine access.

## ADDED Requirements

### Requirement: One supervisor per machine
One runner SHALL supervise all locally configured agents with an exclusive local state-directory lock. It SHALL connect outbound over HTTPS to bot-space and bind local control only to a protected local socket. It SHALL persist its identity, execution journal and session mapping outside project source with owner-only permissions.

#### Scenario: Two local supervisors
- **GIVEN** a running supervisor using a state directory
- **WHEN** another starts with that directory
- **THEN** it exits without claiming work or changing the active supervisor's state.

### Requirement: Local execution configuration
Each managed agent SHALL map to a fixed provider, existing local project directory, credential reference, permitted senders and execution policy. Incoming tasks SHALL not select executable paths, working directories, model credentials or permissions. Default execution SHALL permit analysis; project writes SHALL require explicit local configuration.

#### Scenario: Task attempts to elevate execution
- **GIVEN** a task naming another directory or unrestricted commands
- **WHEN** its configured agent processes it
- **THEN** local policy remains authoritative and unauthorized actions are denied.

### Requirement: Three provider adapters
Runner-managed sessions SHALL support Codex CLI, Claude Code and GitHub Copilot CLI through installed supported versions and machine-local authentication. Startup SHALL report missing binaries, unsupported protocol features and authentication requirements explicitly. Adapters SHALL normalize final text, session identity, failure and interruption without claiming an untested provider works.

#### Scenario: Provider compatibility
- **GIVEN** each configured supported provider
- **WHEN** an agent executes and continues a task
- **THEN** its result and exact persistent session ID are captured through that provider's supported interface.

### Requirement: Automatic task execution and context continuity
The runner SHALL automatically claim eligible tasks, execute the configured agent and return its actual output. A sender's delegation SHALL deliver the returned output into its current turn or exact saved session before continuing the originating work. Sessions SHALL not resume by an ambiguous most-recent selector or attach to unmanaged interactive terminals.

#### Scenario: Continued initiating session
- **GIVEN** an initiating agent with prior task context
- **WHEN** another machine returns a delegated result
- **THEN** the initiating agent continues with both its prior context and the returned result without manual message copying.

### Requirement: Local delegation bridge
Managed providers SHALL receive a local MCP delegate_task tool through the official SDK. It SHALL bind calls to the current managed agent/job, submit idempotent remote tasks and wait locally while keeping remote calls bounded. Provider tool timeouts SHALL preserve a pending delegation for automatic continuation instead of treating missing results as success.

#### Scenario: Remote task exceeds one HTTP call
- **GIVEN** a task taking several minutes
- **WHEN** its sender delegates through the local tool
- **THEN** short remote calls track it and completion is delivered locally or resumed automatically after a provider timeout.

### Requirement: Bounded supervision and recovery
The runner SHALL serialize turns in each session and project with one active execution per machine. It SHALL renew leases independently of model waits, persist completion before delivery, stop process trees on cancellation and recover pending results after restart. Unsafe interrupted executions SHALL require owner review.

#### Scenario: Restart and cancelled process
- **GIVEN** pending delivery or a cancelled running task
- **WHEN** the supervisor restarts or observes cancellation
- **THEN** durable delivery is reconciled and cancelled work cannot keep running unnoticed.

### Requirement: Local secret and output handling
Provider credentials SHALL remain machine-local; agent tokens SHALL be available only to runner transport and protected configuration. Prompts/results SHALL not be placed in command arguments or operational logs. Child input, output and error parsing SHALL be bounded; output overflow SHALL fail explicitly. Provider permission bypass SHALL not be enabled globally.

#### Scenario: Private task diagnostics
- **GIVEN** a task with recognizable private sentinels
- **WHEN** process arguments, operational logs and public configuration are inspected
- **THEN** they expose no task bodies, agent tokens or provider secrets.
