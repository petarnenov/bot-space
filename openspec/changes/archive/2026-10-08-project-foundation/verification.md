# Foundation Verification

Verified locally on 2026-10-08 against Go 1.27.2, PostgreSQL 18.6, Docker 29.5.2, and Compose 5.1.3. Foundation approval and repository identity are recorded in `approval.md`.

## Executed Checks

- `go mod verify`, `go vet ./...`, `go build ./...`: pass.
- Formatting: `test -z "$(gofmt -l cmd internal migrations tests)"`: pass.
- `go test -count=1 ./...` with a real PostgreSQL test server: pass.
- `go test -race -count=1 ./...` with real PostgreSQL: pass; no race reports.
- `govulncheck` v1.8.0: no vulnerabilities found after upgrading `golang.org/x/text` to v0.41.0.
- Strict OpenSpec validation: pass.
- `docker build -t bot-space:local .`: pass. Runtime user is `65532:65532`; exported runtime contains `/mailbox` and trusted CA certificates, with no local `.env`.
- `docker compose config --quiet`: pass.
- `python3 scripts/foundation-smoke.py`: pass, including container replacement, named-volume persistence across ordinary down/up, database outage/recovery, and private-route 404 behavior.
- Railway TOML validated against the official JSON Schema using jsonschema 4.25.1. Schema SHA-256: `0302fd53109298d9c277dbaedae772630506d8da43636e69875268a8782dd68f`.
- CI YAML parses, actions are pinned to inspected commit SHAs, all required commands propagate failures, and no deployment step exists.
- `git check-ignore .env` succeeds; `.env.example` is trackable. Staged-source inspection excludes the synthetic local password.
- Real CLI `migrate` succeeds twice; `serve` responds on configured ports 19000 and 9000. SIGTERM exits zero and PostgreSQL reports no remaining connections with the test application's name.

## Fresh Checkout

Cloned local commit `da7285d` into an independent temporary checkout. Created a fresh local `.env`, separate Compose project, and new database volume. Executed README startup, health/readiness, module verification, vet, all tests, race detector, build, vulnerability scan, and strict OpenSpec validation: all passed. Removed only the disposable check's containers and volume afterward.

This proves the foundation recipe with the documented prerequisites. It does not prove future mailbox, authentication, live GitHub, hosted CI, or Railway behavior.

## Requirement and Scenario Evidence

| Requirement | Scenario evidence |
| --- | --- |
| Environment configuration | `TestLoad` covers defaults, configured 9000/2, missing URL, invalid URL/driver options, port range/text, and pool limits. `TestInvalidConfigurationFailsSafely` verifies nonzero CLI exit without secret output. Actual CLI serving at port 9000 verifies the configured listener. |
| Liveness and readiness | `TestOperationalRoutes`, `TestReadyDeadline`, `TestReadinessHTTPAndPoolExhaustion`, and `TestUnavailableDatabaseHTTP`; Compose smoke verifies real database outage returns 503 while liveness stays 200. Missing migration ledger gives readiness 503; migrated schema gives 200. |
| Bounded HTTP handling | `TestBodyLimit` rejects known-length and chunked oversized input before the next handler; `TestSlowHeaders` exercises the actual five-second socket timeout. Source config explicitly sets 30-second read, 60-second write/idle timeouts. |
| Private features remain unavailable | `TestOperationalRoutes` and live smoke check return 404 for `/mcp` and management routes, without a tool catalog. |
| Safe operational logging | `TestSafeLogs`, `TestLoad`, and CLI failure test check secret sentinels across headers, query, body, config, and underlying errors. Application emits only sanitized operational events. |
| Graceful shutdown | `TestSignalShutdown/complete` exercises SIGINT with an in-flight request that completes; `/deadline` exercises SIGTERM with a hung request cancelled after the real 20-second deadline. Draining readiness is covered by `TestOperationalRoutes`. Actual CLI SIGTERM exits zero and closes real database connections. |
| Durable PostgreSQL state | Compose smoke inserts a committed synthetic fixture, replaces the app without copying its filesystem, and checks the fixture. Ordinary down/up also preserves it. |
| Explicit migration command | `TestMigrateConcurrentAndRepeat`, initial readiness check, and actual repeated CLI/Compose migration invocations; HTTP serving performs only schema validation. |
| Serialized and atomic migrations | `TestMigrateConcurrentAndRepeat` runs two commands simultaneously. `TestMigrationRollback` proves both failed SQL and its version entry roll back while earlier changes survive. `TestMigrationLockDeadline` verifies bounded lock wait with a short test context and asserts the production five-minute default. |
| Migration integrity | `TestMigrationIntegrity/checksum` and `/unknown` prove migration refusal and not-ready behavior for changed checksums and newer versions. Ledger references are schema-qualified; concurrency tests cover the role-dependent search-path regression. |
| Bounded database connections | `TestLoad` rejects invalid limits. `TestReadinessHTTPAndPoolExhaustion` holds every connection and verifies 503 within the combined deadline. |
| Reproducible non-root container | Verified module locks, registry image digests, successful build, runtime UID, binary and CA trust export inspection, and live health endpoints. |
| Local persistent development environment | Compose smoke and independent clean-check stack verify migrations precede ready serving; named volume persists across ordinary stop/start. README distinguishes destructive `down -v`. |
| Railway deployment preparation | Official schema validation; runtime image successfully runs `migrate` and `serve`; TOML uses Dockerfile, explicit migration, configured readiness, environment port, and bounded pre-deploy timeout. |
| Foundation CI checks | Local execution of every workflow check, YAML review of failure propagation, inspected action SHAs, configured PostgreSQL service and test URL. |
| Public repository documentation | MIT, README, CONTRIBUTING, SECURITY, version record, operations docs, and placeholder `.env.example` present. Independent clean-check recipe passes. Local secrets are ignored and absent from staged source. |

## Verification Limits

No live GitHub OAuth flow, hosted GitHub Actions run, public GitHub publication, paid resource provisioning, or Railway deployment has been executed. GitHub and Railway access configuration remains future deployment work. The two-client MCP request/reply gate belongs to change 4 and cannot be claimed from foundation tests. Production backup and restore remain unexecuted.
