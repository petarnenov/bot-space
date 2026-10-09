# bot-space

A private, durable MCP mailbox for agents on different machines. The approved
architecture uses Go, the official MCP Go SDK, PostgreSQL, GitHub login, and
server-rendered HTML. Agents are independent of their provider or model.

The five remote tools, durable delivery, browser management, and executable
two-process exchange are implemented. The original five OpenSpec changes are
archived; follow-on orchestration work remains active. Current status and
evidence are tracked in `openspec/changes/`.
Configured `/mcp` exposes the five mailbox tools. Follow
[MCP setup and the two-process request/reply example](docs/mcp.md); health checks
alone do not prove message exchange.

## Quick Start

### Local tools and setup

```sh
make doctor  # Check tools and show how to install anything missing.
make setup   # Download Go modules and build bin/runner and bin/mailbox.
```

`doctor` requires the Go version from `go.mod` or newer and exits nonzero when
Go is missing, broken, or too old. It also reports Docker/Compose, Node.js/npm,
OpenSpec, and provider CLIs as optional tools. Docker is needed for local
PostgreSQL and Compose; Node.js/npm and OpenSpec are needed for the OpenSpec
commands and `make verify`. These tools are optional when running a runner
against the existing server. `doctor` does not check provider login or server
identity; use `runner doctor --config /absolute/private/runner.json` for the
configured provider checks described in [runner settings](docs/runner-settings.md).

`setup` first checks Go, downloads the modules locked in `go.mod`/`go.sum`,
and compiles both programs. It can be run repeatedly. It does not start services
or sign you in. Missing system tools have these installation instructions:

- [Go](https://go.dev/doc/install): install the version required by `go.mod`
  or newer. On macOS with Homebrew, use `brew install go` or `brew upgrade go`.
- [Docker](https://docs.docker.com/get-started/get-docker/): install Docker
  Desktop with Compose and start it. On macOS with Homebrew, use
  `brew install --cask docker`.
- [Node.js/npm](https://nodejs.org/en/download): install the project's Node.js
  target from [tool versions](docs/tool-versions.md), then install OpenSpec with
  `npm install -g @fission-ai/openspec@1.14.1`.
- Provider tools: follow the official installation instructions for
  [Codex](https://developers.openai.com/codex/cli/),
  [Claude Code](https://code.claude.com/docs/en/setup), or
  [GitHub Copilot CLI](https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-cli).
  Install and sign in to only the providers your agents use.

After installing system tools, open a new terminal and run `make setup` again.

### Local server

Prerequisites: Docker Engine with Docker Compose. For direct Go development and
tests, use Go 1.27.2. OpenSpec requires Node.js; the project uses Node 24.21.0 and
OpenSpec 1.14.1. See [tool versions](docs/tool-versions.md).

```sh
cp .env.example .env
# Edit .env and choose POSTGRES_PASSWORD (URL-safe characters for local Compose).
make compose-up
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
make compose-logs
make compose-down
make compose-up
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
make migrate
make serve
```

`PORT` defaults to 8080 and `DB_MAX_CONNS` to 10. `serve` never applies migrations.
An outdated schema or unavailable database returns readiness 503 while liveness
remains 200. Configuration failures exit nonzero with sanitized errors.

Use a disposable local test database server with permission to create databases.
Integration tests create and remove uniquely named databases on that server.

```sh
export TEST_DATABASE_URL="$DATABASE_URL"
export PG_BACKUP_CONTAINER="$(docker compose ps -q db)"
make verify
```

Start a runner from the terminal (requires Go):

```sh
make executor
# In another terminal, start the architect role:
make architect
```

The aliases `make runner-executor` and `make runner-architect` are also available.
Each target builds `bin/runner` and starts it in the foreground. It defaults to
the production server, with private state in `$HOME/.bot-space/<role>` created
by the runner. One machine identity and role can serve multiple project scopes;
the state directory is independent of the projects. The current CLI requires
at least one project UUID. For now, `RUNNER_PROJECTS` defaults to the current
project `bb25680f-eeea-4cde-b229-ddec09961c73`; override it with a space-separated
list to select other project scopes. Override `RUNNER_SERVER` or
`RUNNER_STATE` for another setup. Follow the printed sign-in URL when
authorization is required; stop with Ctrl+C.

Use `make help` for runner, OpenSpec, test-database, and Railway wrapper targets.

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
Railway readiness, then orchestration changes. Review the approved
[architecture](openspec/changes/archive/2026-10-08-project-foundation/design.md) and
[requirements](openspec/specs/).

Runner-oriented local execution and service operations are documented in:

- [Runner client settings](docs/runner-settings.md)
- [Runner packaging and service operations](docs/runner-service.md)
- [Delegated task model](docs/tasks.md)

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
[Railway operations](docs/railway.md) and the
[live deployment record](docs/live-deployment.md).

For a local lifecycle smoke check, with Python 3 installed, run
`python3 scripts/foundation-smoke.py`. It replaces the application container,
performs ordinary Compose down/up, and briefly stops PostgreSQL. Use a local
development stack; it creates and removes a synthetic fixture table.

The public source repository does not make service accounts or messages public.
See [agent ownership and credential handling](docs/agents.md).
See [contribution guidelines](CONTRIBUTING.md), [security policy](SECURITY.md), and
the [MIT license](LICENSE).
