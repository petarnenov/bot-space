# Tasks

Implementation starts only after user approval of the first specification and architecture. Public Go module initialization also requires confirmed GitHub owner/repository. All tasks below are pending; creating these documents does not complete any implementation task.

## 1. Project and Toolchain

- [x] 1.1 Record user approval and confirmed project name/GitHub owner/repository; verify the recorded scope matches `proposal.md` before source changes.
- [ ] 1.2 Initialize the approved Go module using Go 1.27.2 and pgx v5.11.0; verify `go version`, `go mod verify`, and `go list -m all`, and commit dependency locks. Introduce MCP SDK v1.8.0 with its actual usage in change 4.
- [x] 1.3 Establish `cmd/mailbox`, configuration, database, and HTTP packages; verify `go build ./...` succeeds without placeholder product endpoints.
- [x] 1.4 Add MIT licensing, CONTRIBUTING.md, SECURITY.md, and a tool version record; verify all files exist, license attribution is appropriate, and no unexecuted validation is described as successful. Preserve existing `AGENTS.md`.

## 2. Service Runtime

- [x] 2.1 Implement environment parsing, port defaults/range checks, pool limits, and sanitized startup failures; add tests for valid and invalid inputs and verify `go test ./internal/config/...`.
- [x] 2.2 Implement health/readiness with a combined two-second database/schema deadline and bounded JSON; test unavailable database, missing migrations, and shutdown state over HTTP, and verify corresponding integration cases.
- [x] 2.3 Implement request body and server timeouts, private-route 404 behavior, and secret-safe logging; verify oversized body, incomplete headers, premature `/mcp`, and sensitive-sentinel tests pass.
- [x] 2.4 Implement SIGINT/SIGTERM drain, cancellation, and pool closure; verify subprocess tests cover completed and deadline-exceeded requests within the specified shutdown budget.
- [ ] 2.5 Document `mailbox serve`, environment settings, health behavior, and operational limits; run the documented commands and verify responses match `service-runtime`.

## 3. Database Lifecycle

- [x] 3.1 Implement pooled PostgreSQL access and migration ledger with embedded numbered SQL; verify real-PostgreSQL tests cover connection exhaustion, missing schema, and explicit migration behavior.
- [x] 3.2 Implement `mailbox migrate` with advisory locking, per-migration transactions, checksums, and a five-minute total deadline; verify concurrent migrators, repeat execution, rollback, altered checksums, and unknown versions against PostgreSQL.
- [x] 3.3 Verify migration lock timeout using a short configurable test deadline with the production five-minute default separately checked; assert a nonzero sanitized failure and no partial migration.
- [x] 3.4 Add database integration test fixtures and a persistence check across application replacement; verify committed fixtures survive without application filesystem copying.
- [ ] 3.5 Document migrations, compatibility restrictions, connection budgets, and forward-repair strategy; verify commands work against a clean database and rerun safely.

## 4. Containers and Railway Preparation

- [x] 4.1 Resolve and record exact Go/PostgreSQL/runtime image digests, build the multi-stage non-root image with CA roots, and verify runtime UID, executable subcommands, and health endpoints.
- [x] 4.2 Add Compose with a named database volume, database healthcheck, explicit migrator, and loopback host publishing; verify fresh startup, ordinary down/up persistence, and replacement of the application container.
- [x] 4.3 Add `.env.example` placeholders and ignore rules; verify `git check-ignore .env` and that the example remains trackable, with no real credentials in staged files.
- [x] 4.4 Prepare `railway.toml` with Docker build, migration pre-deploy command, and `/readyz`; validate settings against current official Railway schema/docs and verify the migration command runs from the built image. Do not provision or deploy.
- [ ] 4.5 Write and execute README foundation quick-start and container commands from a clean temporary checkout; record the result and document pending OAuth/mailbox functionality, callback/domain requirements, and the unverified live-deployment status.

## 5. CI and Integrated Verification

- [x] 5.1 Add GitHub Actions with SHA-pinned actions, locked Go/OpenSpec/scanner versions, and real PostgreSQL; run its command equivalents locally: formatting, `go vet ./...`, tests, `go test -race ./...`, `go build ./...`, vulnerability scanning, and `openspec validate --all --strict --no-interactive`.
- [x] 5.2 Verify each CI job propagates command failure and that the workflow has no deployment step; record local verification separately from hosted GitHub Actions execution.
- [ ] 5.3 Complete the requirement matrix below with actual test names, commands, and outcomes in `verification.md`; verify every requirement and scenario has evidence, and explicitly identify any unexecuted external check.
- [x] 5.4 Run foundation smoke checks in Compose after ordinary shutdown/restart and application replacement; verify readiness, liveness, migration integrity, persistence, and sanitized logs together.

## Requirement-to-Check Matrix

| Capability requirement | Verification coverage |
| --- | --- |
| service-runtime: Environment configuration | 2.1, 2.5; startup exit/listener assertions |
| service-runtime: Liveness and readiness | 2.2, 5.4; real HTTP and PostgreSQL failures |
| service-runtime: Bounded HTTP handling | 2.3; body overflow and slow-header tests |
| service-runtime: Private features remain unavailable | 2.3; `/mcp` and management-route requests |
| service-runtime: Safe operational logging | 2.1, 2.3, 5.4; sentinel checks in logs/errors |
| service-runtime: Graceful shutdown | 2.4; signal/subprocess tests |
| database-lifecycle: Durable PostgreSQL state | 3.4, 4.2, 5.4; committed fixture after replacement |
| database-lifecycle: Explicit migration command | 3.1, 3.2, 3.5; empty schema and repeat invocation |
| database-lifecycle: Serialized and atomic migrations | 3.2, 3.3; concurrency, rollback, deadline |
| database-lifecycle: Migration integrity | 3.2; modified checksum and unknown version |
| database-lifecycle: Bounded database connections | 2.1, 3.1; pool exhaustion and configuration |
| deployment-runtime: Reproducible non-root container | 1.2, 4.1; locks, digests, UID, HTTP smoke |
| deployment-runtime: Local persistent development environment | 4.2, 5.4; Compose restart |
| deployment-runtime: Railway deployment preparation | 4.4; schema/docs review and runtime command |
| deployment-runtime: Foundation CI checks | 5.1, 5.2; equivalent commands and job exit review |
| deployment-runtime: Public repository documentation | 1.4, 4.3, 4.5; fresh setup and secret exclusion |

## Workflow follow-up

- Archive only after all tracked tasks and requirement/scenario checks succeed; apply the installed OpenSpec archive workflow and verify the resulting main specs.
- Create the next sequential change, `identity-workspaces`, with proposal, specs, design, and tasks before implementation.
- Keep live Railway deployment pending explicit user authorization and report it as unverified until actually executed.
