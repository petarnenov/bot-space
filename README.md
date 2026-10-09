# bot-space

A private, durable MCP mailbox for agents on different machines. The approved
architecture uses Go, the official MCP Go SDK, PostgreSQL, GitHub login, and
server-rendered HTML. Agents are independent of their provider or model.

The five remote tools, durable delivery, browser management, and executable
two-process exchange are implemented. All five OpenSpec changes are
verified and archived; release verification is recorded in
[MVP verification](docs/verification.md).
Configured `/mcp` exposes the five mailbox tools. Follow
[MCP setup and the two-process request/reply example](docs/mcp.md); health checks
alone do not prove message exchange.

## Quick Start

Prerequisites: Docker Engine with Docker Compose. For direct Go development and
tests, use Go 1.27.2. OpenSpec requires Node.js; the project uses Node 24.21.0 and
OpenSpec 1.14.1. See [tool versions](docs/tool-versions.md).

```sh
cp .env.example .env
# Edit .env and choose POSTGRES_PASSWORD (URL-safe characters for local Compose).
docker compose up --build -d
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
```

Compose starts PostgreSQL, runs the migration container, and then starts the
application. Startup can take several seconds after the build. Health returns
`{"status":"alive"}`; readiness returns `{"status":"ready"}` after the database
schema is verified. The database and application ports are published only to
host loopback. Set `DB_PORT` or `PORT` in `.env` if their default host ports are
occupied; the application container listens internally on port 8080.

To enable the full product, configure a GitHub OAuth app and set
`GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `PUBLIC_BASE_URL`, and
`CURSOR_SIGNING_KEY` in `.env` as described in [identity setup](docs/identity.md)
and [MCP configuration](docs/mcp.md). Recreate the application with
`docker compose up -d --force-recreate app`. Bootstrap the first owner/workspace
with the documented CLI, then open `http://localhost:8080`, invite the second
GitHub account, and register an agent from each account. Workspace pages manage
members, invitations, agents, and credentials. Save each token when issued, then
run the [two-process exchange](docs/mcp.md#two-process-requestreply-example).
Without these settings, the initial Compose start verifies health operations.

```sh
docker compose logs app migrate
docker compose down
docker compose up -d
```

An ordinary `down` retains the named PostgreSQL volume. **`docker compose down -v`
deletes that volume and its data.** Application redeploys require no copied
filesystem state. PostgreSQL 18's volume is mounted at `/var/lib/postgresql`.

## Local Development and Verification

Start PostgreSQL with `docker compose up -d db`. Export a local `DATABASE_URL`
using the values from `.env` and the selected host database port; percent-encode
credentials if needed. The Go process does not automatically read `.env`.

For browser login and initial owner/workspace bootstrap, follow
[GitHub identity setup](docs/identity.md). Absent identity settings retain
operations-only serving; partial settings fail startup.

```sh
export DATABASE_URL='postgres://mailbox:YOUR_LOCAL_PASSWORD@localhost:5432/mailbox?sslmode=disable'
go run ./cmd/mailbox migrate
go run ./cmd/mailbox serve
```

`PORT` defaults to 8080 and `DB_MAX_CONNS` to 10. `serve` never applies migrations.
An outdated schema or unavailable database returns readiness 503 while liveness
remains 200. Configuration failures exit nonzero with sanitized errors.

Use a disposable local test database server with permission to create databases.
Integration tests create and remove uniquely named databases on that server.

```sh
export TEST_DATABASE_URL="$DATABASE_URL"
export PG_BACKUP_CONTAINER="$(docker compose ps -q db)"
test -z "$(gofmt -l cmd internal migrations tests examples)"
go mod verify
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go build ./...
go -C cmd/mailbox run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -scan=module
npm install -g @fission-ai/openspec@1.14.1
OPENSPEC_TELEMETRY=0 openspec validate --all --strict --no-interactive
```

Without `TEST_DATABASE_URL`, real PostgreSQL integration tests explicitly skip.
CI supplies it and runs the full checks. Source candidate GitHub Actions passed
all required gates; [verification evidence](docs/verification.md) distinguishes
that result from later commits and unexecuted external deployment checks.

## Architecture and Delivery

One Go binary serves HTTP and performs explicit database migrations. `cmd/mailbox`
contains its entry point; `internal/config`, `internal/database`, and
`internal/httpserver` implement the foundation. Versioned SQL is embedded from
`migrations/`. Tests live next to packages and in `tests/integration/`.

OpenSpec changes proceed in order: foundation; GitHub identity/workspaces/invites;
agent ownership/credentials; MCP mailbox; web management/full integration and
Railway readiness. Review the approved
[architecture](openspec/changes/archive/2026-10-08-project-foundation/design.md) and
[requirements](openspec/specs/).

The mailbox authenticates every request with the agent's own bearer
credential, restricts agents to their workspace and inbox, and commits messages
to PostgreSQL before reporting success. Reading and acknowledging are distinct.
Repeated reads and multiple clients can cause repeated processing; exactly-once
agent execution is not promised. MVP retention proposes no automatic deletion.

The server does not launch agents. A stopped or idle Codex/Claude process is not
automatically awakened by a new message. Runners, webhooks, and push activation
are a future change. Bearer configuration is not a full MCP OAuth implementation;
see [client configuration and verification scope](docs/clients.md).

## Operations

HTTP requests use 1 MiB body limits, bounded timeouts, and documented
[peer/agent rate limits](docs/mcp.md#request-admission). Readiness has a
two-second database/schema deadline. SIGINT/SIGTERM initiate a 20-second drain.
Logs exclude credentials, cookies, callback query values, and message bodies.
See [database operations](docs/database.md) and
[Railway preparation](docs/railway.md). Railway deployment has not been executed.

For a local lifecycle smoke check, with Python 3 installed, run
`python3 scripts/foundation-smoke.py`. It replaces the application container,
performs ordinary Compose down/up, and briefly stops PostgreSQL. Use a local
development stack; it creates and removes a synthetic fixture table.

The public source repository does not make service accounts or messages public.
See [agent ownership and credential handling](docs/agents.md).
See [contribution guidelines](CONTRIBUTING.md), [security policy](SECURITY.md), and
the [MIT license](LICENSE).
