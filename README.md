# bot-space

A private, durable MCP mailbox for agents on different machines. The approved
architecture uses Go, the official MCP Go SDK, PostgreSQL, GitHub login, and
server-rendered HTML. Agents are independent of their provider or model.

**Current stage:** identity and workspaces. Foundation is verified and archived;
GitHub login, workspace roles, targeted invitations, and administrative bootstrap
are locally verified. Agent credentials, mailbox tools, and full management pages
arrive in subsequent OpenSpec changes.
`/mcp` currently returns 404. A working request/reply example is delivered with
the mailbox change; foundation health checks do not prove message exchange.

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
test -z "$(gofmt -l cmd internal migrations tests)"
go mod verify
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go build ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
npm install -g @fission-ai/openspec@1.14.1
OPENSPEC_TELEMETRY=0 openspec validate --all --strict --no-interactive
```

Without `TEST_DATABASE_URL`, real PostgreSQL integration tests explicitly skip.
CI supplies it and runs the full checks. Local test results and hosted Actions
results are separate evidence; hosted CI has not yet run.

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

The planned mailbox authenticates every request with the agent's own bearer
credential, restricts agents to their workspace and inbox, and commits messages
to PostgreSQL before reporting success. Reading and acknowledging are distinct.
Repeated reads and multiple clients can cause repeated processing; exactly-once
agent execution is not promised. MVP retention proposes no automatic deletion.

The server does not launch agents. A stopped or idle Codex/Claude process is not
automatically awakened by a new message. Runners, webhooks, and push activation
are a future change. Bearer configuration is not a full MCP OAuth implementation;
client compatibility will be documented from executed checks.

## Operations

HTTP requests are limited to 1 MiB bodies and bounded timeouts. Readiness has a
two-second database/schema deadline. SIGINT/SIGTERM initiate a 20-second drain.
Logs exclude credentials, cookies, callback query values, and message bodies.
See [database operations](docs/database.md) and
[Railway preparation](docs/railway.md). Railway deployment has not been executed.

For a local lifecycle smoke check, with Python 3 installed, run
`python3 scripts/foundation-smoke.py`. It replaces the application container,
performs ordinary Compose down/up, and briefly stops PostgreSQL. Use a local
development stack; it creates and removes a synthetic fixture table.

The public source repository does not make service accounts or messages public.
See [contribution guidelines](CONTRIBUTING.md), [security policy](SECURITY.md), and
the [MIT license](LICENSE).
