# Runner Identity

## Purpose

Automatically verify physical machine supervisors through GitHub collaborator identity and enforce their control roles independently of local model-provider accounts.

## ADDED Requirements

### Requirement: GitHub-backed machine enrollment
A runner SHALL authenticate with GitHub at startup and prove its machine key. The server SHALL automatically admit only the repository owner or an explicit GitHub collaborator, without a bot-space invitation or manual grant. Public read access SHALL not qualify. The runner SHALL select architect or executor; GitHub client secrets SHALL remain server-side.

#### Scenario: Collaborator enrollment without invitation
- **GIVEN** an authenticated GitHub collaborator and new machine key
- **WHEN** the runner selects architect or executor at startup
- **THEN** the server verifies repository access and registers it without an additional bot-space invitation.

#### Scenario: Public reader or unverifiable access
- **GIVEN** a public reader or unavailable GitHub access verification
- **WHEN** registration is attempted
- **THEN** the server denies admission rather than equating public visibility with collaborator access.

### Requirement: Revocable scoped machine credentials
Only the runner SHALL hold its central credential. Every RPC/frame mutation SHALL check credential, runner activation, role and current verified repository authority. Refresh SHALL prove the machine key and revalidate GitHub access. Removal of collaborator access SHALL invalidate control authority within a documented bounded refresh interval; unverifiable refresh SHALL fail closed.

#### Scenario: Revocation on an open connection
- **GIVEN** an authenticated stream whose runner is revoked
- **WHEN** another control operation arrives
- **THEN** the stale connection cannot continue privileged work.

### Requirement: Role identity and provider separation
Each registered runner SHALL have an explicit architect or executor role and one authorized council vote identity. Provider brand, model sessions or advertised capabilities SHALL not create extra votes or elevate roles. Codex, Claude and Copilot authentication SHALL remain machine-local and separate from bot-space enrollment.

#### Scenario: Provider does not elevate identity
- **GIVEN** one registered executor running multiple local model sessions
- **WHEN** a model attempts to submit an architect vote
- **THEN** its executor identity remains unchanged and the vote is denied.

### Requirement: Startup client and inference configuration
Startup SHALL select client, default model and effort per runner, with CLI precedence over private configuration. Adapters SHALL validate settings without silent substitution and preserve exact-session continuation, bounded results, local permissions and credential separation. Codex, Claude, Copilot, Hermes and OpenClaw SHALL have verified adapters; unsupported effort controls SHALL fail clearly.

#### Scenario: Independent client settings
- **GIVEN** an architect and executor running on one host
- **WHEN** they start with different provider, model and effort settings
- **THEN** each uses its selected configuration without changing the other runner or adding voting identities.

#### Scenario: Architect and executor on one physical host
- **GIVEN** a collaborator starting an architect and an executor on the same machine
- **WHEN** each uses its own private state directory and role-bound identity
- **THEN** both runners operate concurrently without sharing credentials, confusing sessions or adding a second executor slot.

#### Scenario: Multiple local models
- **GIVEN** one architect runner using several model sessions
- **WHEN** opinions or votes are submitted
- **THEN** that registered member has one vote and cannot multiply the council quorum.

### Requirement: Defaults and startup rejection with choices
When model or effort is omitted, the runner SHALL preserve the selected client's own default for that setting. Explicit settings SHALL be checked against the client's current model catalog and model-specific effort capabilities before starting any work session. Invalid model/effort SHALL prevent startup and display the available valid choices. No silent fallback SHALL occur. An unverifiable catalog SHALL produce a clear configuration error rather than assume a requested combination is valid.

#### Scenario: Omitted inference settings
- **GIVEN** a selected local coding client with its own configuration
- **WHEN** startup omits model and effort overrides
- **THEN** no overrides are injected and the client uses its own defaults.

#### Scenario: Invalid explicit inference setting
- **GIVEN** an explicitly selected model or effort unsupported by the chosen client/model
- **WHEN** startup validation runs
- **THEN** no work session starts and the error lists the valid available model/effort choices.
