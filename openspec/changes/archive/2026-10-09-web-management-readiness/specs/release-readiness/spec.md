# Release Readiness

## Purpose

Prove the complete private mailbox from a fresh checkout and prepare auditable public-source CI and Railway operation without an unauthorized deployment.

## ADDED Requirements

### Requirement: Complete browser to MCP acceptance
A real-PostgreSQL acceptance check SHALL use mocked GitHub HTTP login for two identities, protected invitation acceptance, own-agent registration and separate credentials, followed by independent HTTP SDK send/read/ack/reply/read. It SHALL verify foreign workspace denial, revoked credentials and persistence after restart, without substituting only direct Go function calls.

#### Scenario: Complete invited-user exchange
- **GIVEN** two GitHub identities and an isolated third workspace
- **WHEN** browser and MCP acceptance flows run
- **THEN** both intended members exchange messages, isolation/revocation hold, and committed data persists.

### Requirement: Verified client and activation documentation
Documentation SHALL record actual client/tool versions and distinguish configuration parsing/connection checks from model-driven tool execution. Go SDK client exchange SHALL be executed; Codex/Claude bearer support SHALL be checked through installed CLI/official docs and any available connection probe. Unexecuted claims, full MCP OAuth, automatic wakeup and single-consumer guarantees SHALL NOT be advertised.

#### Scenario: Reproducible client configuration
- **GIVEN** the documented tested client versions/configuration
- **WHEN** a contributor repeats the stated check
- **THEN** its claimed scope matches actual evidence and secrets stay outside public files.

### Requirement: Backup restore and Railway preparation
Runbooks SHALL cover domain/TLS, exact OAuth callback, required environment secrets, database backup/isolated restore, graceful shutdown and migration strategy. A local PostgreSQL backup/restore drill SHALL verify restored ledger and mailbox/credential state. Railway schema/start/predeploy/health settings SHALL be reviewed against official docs, and live deployment SHALL remain unverified unless explicitly authorized and executed.

#### Scenario: Isolated restore drill
- **GIVEN** a synthetic complete database backup
- **WHEN** it is restored into a separate empty database
- **THEN** schema, durable messages and credential state are verified without touching a live production database.

### Requirement: Public source and CI evidence
The existing public GitHub repository SHALL contain the complete source, MIT license, contributor/security docs, placeholder config and required CI checks, without real credentials/data. After local verification, its GitHub Actions workflow SHALL pass formatting, vet, real-DB tests/race, build, module vulnerability scan and OpenSpec validation. Hosted results SHALL be tied to the actual source commit.

#### Scenario: Public release candidate
- **GIVEN** reviewed source and no secret files staged
- **WHEN** the requested repository changes are published and CI runs
- **THEN** required checks pass for that commit and no Railway deployment or paid provisioning occurs.

### Requirement: Full fresh checkout reproduction and audit
README commands SHALL reproduce the complete local service with documented prerequisites and owner-supplied OAuth configuration. Final verification SHALL map every original requirement and spec scenario to current evidence, retain external verification limits, and archive only after all tasks/requirements pass. Existing AGENTS.md SHALL remain unchanged.

#### Scenario: Final readiness review
- **GIVEN** an independent clean checkout
- **WHEN** documented local verification and completion audit run
- **THEN** the complete requested behavior is proven, unknown external checks are stated honestly, and no required local work is omitted.
