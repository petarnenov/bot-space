# Proposal

## Why

Agents on separate machines need a durable, private mailbox independent of their model provider. This first change establishes a reproducible Go/PostgreSQL runtime and operational checks so later authentication and mailbox changes can be verified safely.

## What Changes

- Establish a Go application skeleton, versioned PostgreSQL migrations, pooled database access, environment configuration, and graceful shutdown.
- Add `/healthz` and `/readyz`; keep `/mcp` and all product features unavailable until authenticated transport is implemented.
- Provide a multi-stage, non-root Docker image, local Compose environment, and foundation CI checks.
- Introduce MIT licensing, contributor/security documentation, a tool version record, and an implementation verification matrix.
- Prepare Railway deployment configuration with a migration pre-deploy command; do not deploy or provision resources.
- Record the complete MVP architecture and ordered delivery plan for review.

Scope: foundation only. GitHub login, workspace bootstrap, invitations, agents, credentials, mailbox tools, and management pages are subsequent changes. A working foundation is not a completed mailbox product.

## Capabilities

### New Capabilities

- `service-runtime`: configuration validation, safe HTTP operation, health, readiness, and shutdown.
- `database-lifecycle`: durable PostgreSQL connectivity, serialized versioned migrations, and restart behavior.
- `deployment-runtime`: reproducible containers, local development, Railway preparation, and verification.

### Modified Capabilities

None; no existing product specs or application code are present.

## Impact

Proposed implementation paths: `cmd/mailbox/`, `internal/`, `migrations/`, `tests/integration/`, `Dockerfile`, `compose.yaml`, `railway.toml`, and `.github/workflows/ci.yml`. Dependencies: Go, PostgreSQL, official MCP Go SDK, and pgx. OpenSpec is development tooling only; there is no Node frontend or runtime. The user confirmed `petarnenov/bot-space`; the public Go module path is `github.com/petarnenov/bot-space`.
