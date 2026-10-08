# service-runtime Specification

## Purpose

Provide predictable startup, health reporting, request handling, and shutdown for the private mailbox service.

## Requirements

### Requirement: Environment configuration
The service SHALL load secrets only from environment variables, require a valid PostgreSQL `DATABASE_URL`, and listen on `0.0.0.0:$PORT`, defaulting to port 8080 when `PORT` is absent. Invalid ports, missing database configuration, and invalid configuration SHALL cause a nonzero exit with a sanitized error before serving requests.

#### Scenario: Railway supplied port
- **GIVEN** valid configuration and `PORT=9000`
- **WHEN** the service starts
- **THEN** it accepts HTTP connections on all interfaces at port 9000.

#### Scenario: Invalid configuration
- **GIVEN** a missing `DATABASE_URL` or a port outside 1–65535
- **WHEN** the service starts
- **THEN** it exits nonzero without exposing secret values or opening its HTTP listener.

### Requirement: Liveness and readiness
`GET /healthz` SHALL return HTTP 200 when the HTTP service is running. `GET /readyz` SHALL return HTTP 200 only when a database ping succeeds within two seconds, all required migrations are applied, and shutdown has not started; otherwise it SHALL return HTTP 503. Both endpoints SHALL return bounded JSON without credentials, database details, or private data.

#### Scenario: Database unavailable after startup
- **GIVEN** the HTTP listener is running and database connectivity fails
- **WHEN** both health endpoints are requested
- **THEN** `/healthz` returns 200 and `/readyz` returns 503 within two seconds plus scheduling tolerance.

#### Scenario: Required schema missing
- **GIVEN** PostgreSQL responds but a required migration is absent
- **WHEN** readiness is requested
- **THEN** `/readyz` returns 503.

### Requirement: Bounded HTTP handling
The service SHALL reject request bodies above 1 MiB with HTTP 413 before application decoding and configure a five-second header timeout, 30-second read timeout, 60-second write timeout, and 60-second idle timeout. No application endpoint SHALL accept unlimited input.

#### Scenario: Oversized body
- **GIVEN** a request body larger than 1 MiB
- **WHEN** it reaches an application handler
- **THEN** the service returns 413 without processing the body.

#### Scenario: Incomplete headers
- **GIVEN** a client that does not complete headers within five seconds
- **WHEN** the header deadline expires
- **THEN** the server terminates the request without invoking an application handler.

### Requirement: Private features remain unavailable
Configured identity routes SHALL be available after identity initialization. Independently configured mailbox routes SHALL expose authenticated MCP at `/mcp`; without mailbox configuration that route SHALL return 404. Unimplemented management routes SHALL remain unavailable. If neither identity nor mailbox is configured, only public health endpoints SHALL be available.

#### Scenario: Premature MCP access
- **GIVEN** the foundation service is running
- **WHEN** a client sends an MCP initialization request to `/mcp`
- **THEN** it receives 404 and no MCP tool catalog.

#### Scenario: Configured identity routes
- **GIVEN** valid GitHub identity configuration
- **WHEN** a browser requests login or uses its valid session for workspace selection
- **THEN** the configured identity flow is available and only its own memberships are shown.

#### Scenario: Configured mailbox routes
- **GIVEN** valid mailbox cursor-signing configuration
- **WHEN** a native client connects with its own valid bearer credential
- **THEN** authenticated MCP is available independently of browser login.

### Requirement: Safe operational logging
Logs SHALL contain operational metadata only and SHALL exclude bearer tokens, OAuth codes/tokens, cookies, invitation secrets, database credentials, request query values, and message bodies. Client errors SHALL exclude raw database errors and configuration values.

#### Scenario: Sensitive failure input
- **GIVEN** a failing request or configuration containing recognizable secret sentinel values
- **WHEN** diagnostic logs and client errors are collected
- **THEN** neither output contains the sentinels or request body.

### Requirement: Graceful shutdown
On SIGTERM or SIGINT, the service SHALL mark readiness unavailable, stop accepting new requests, allow up to 20 seconds for in-flight requests, close database resources, and exit. It SHALL cancel remaining requests when the drain deadline expires.

#### Scenario: Shutdown during a request
- **GIVEN** an in-flight request that completes within the drain deadline
- **WHEN** SIGTERM is delivered
- **THEN** the request can finish and the process exits after releasing database resources.

#### Scenario: Shutdown deadline exceeded
- **GIVEN** a request that cannot complete within 20 seconds
- **WHEN** shutdown begins
- **THEN** the process cancels outstanding work and exits within the drain budget plus scheduling tolerance.
