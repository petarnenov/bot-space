# Deployment Runtime

## Purpose

Make the foundation reproducible on a fresh development machine and prepare an auditable Railway deployment without publishing it.

## ADDED Requirements

### Requirement: Reproducible non-root container
The project SHALL provide a multi-stage `Dockerfile` using an exact stable Go version and a runtime with a non-root user and trusted CA certificates. Dependency versions SHALL be locked in `go.mod` and `go.sum`; image references SHALL be pinned by digest at implementation time. The runtime SHALL execute the same binary for HTTP serving and migrations.

#### Scenario: Build and run
- **GIVEN** the documented prerequisites and committed dependency locks
- **WHEN** the Docker image is built and started with valid configuration
- **THEN** it runs as a non-root user and serves the health endpoints.

### Requirement: Local persistent development environment
Docker Compose SHALL provide the application and PostgreSQL with a named database volume, database healthcheck, and an explicit migration step before the application becomes ready. Documentation SHALL distinguish an ordinary stop from destructive volume removal.

#### Scenario: Ordinary stop and restart
- **GIVEN** a ready Compose environment containing a committed fixture
- **WHEN** `docker compose down` and the documented startup commands are run
- **THEN** the fixture persists and readiness returns 200 after migration validation.

### Requirement: Railway deployment preparation
Railway configuration SHALL use the Dockerfile, a `mailbox migrate` pre-deploy command, and `/readyz` as the deployment healthcheck. The service SHALL use `DATABASE_URL` and the Railway-supplied `PORT`. Documentation SHALL state that preparation is not a verified deployment and that provisioning or publication requires explicit user permission.

#### Scenario: Prepared configuration review
- **GIVEN** the committed deployment configuration
- **WHEN** its settings are inspected against Railway documentation
- **THEN** build, migration, healthcheck, environment, and startup settings match the documented application behavior and no resources have been provisioned.

### Requirement: Foundation CI checks
GitHub Actions SHALL fail on formatting differences, `go vet` findings, failing unit or real-PostgreSQL integration tests, race detector failures, build failures, dependency vulnerability findings, or strict OpenSpec validation failures. The workflow SHALL use locked tool versions and actions pinned to commit SHAs. It SHALL NOT deploy.

#### Scenario: Verification failure blocks CI
- **GIVEN** a pull request with a failing check
- **WHEN** the verification workflow runs
- **THEN** the corresponding job exits nonzero and no deployment job runs.

### Requirement: Public repository documentation
The project SHALL provide an MIT license, README, CONTRIBUTING.md, SECURITY.md, and `.env.example` containing placeholders. Local secret files SHALL be ignored. Documentation SHALL record tool versions, exact verified commands, known limitations, and pending product features, without claiming that an unexecuted check passed.

#### Scenario: Fresh-machine foundation setup
- **GIVEN** a clean checkout and the documented prerequisites
- **WHEN** the README foundation commands are followed with local placeholder configuration replaced appropriately
- **THEN** the application becomes ready and the documented foundation verification commands succeed.

#### Scenario: Secret file exclusion
- **GIVEN** a local `.env` file with secret values
- **WHEN** Git ignored-file checks run
- **THEN** the file is ignored and `.env.example` remains trackable.
