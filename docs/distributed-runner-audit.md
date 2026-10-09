# Distributed runner audit (current evidence)

This audit covers the `distributed-agent-runner` OpenSpec change against current
source, local CI gates, and recorded live evidence.

- Reviewed source areas: `internal/tasks`, `internal/runner`, `internal/taskweb`,
  `internal/mcpserver/tasks.go`, `cmd/runner`, `cmd/mailbox`.
- Local gates executed in this cycle: `gofmt`, `go vet ./...`, `go test ./...`,
  focused race tests for runner/tasks paths, `go build ./...`,
  `govulncheck ./...`, and `openspec validate --changes "distributed-agent-runner" --strict`.
- Added process-level orchestration proof test:
  `internal/runner/process_integration_test.go:TestTwoIndependentSupervisorsDelegateAndContinue`.
  It now runs three deterministic process paths (`copilot→copilot`,
  `codex→copilot`, `copilot→codex`), each with two independent supervisors,
  local bridge delegation and verified continuation text that includes B's
  actual returned result.
- Real PostgreSQL execution gates were run with Docker-backed
  `TEST_DATABASE_URL=postgres://mailbox:botspace@127.0.0.1:55432/mailbox?sslmode=disable`,
  including `go test ./tests/integration -count=1`.
- AGENTS.md status: unchanged.

## Requirement coverage snapshot

| Capability area | Status | Primary evidence |
| --- | --- | --- |
| Durable task lifecycle/idempotency/fencing | implemented | `internal/tasks/*.go`, `tests/integration/tasks_test.go`, `tests/integration/tasks_http_test.go` |
| Local runner config/locking/supervision | implemented | `internal/runner/*.go`, `internal/runner/*_test.go`, `cmd/runner/main.go` |
| Local delegation bridge and continuation journal | implemented | `internal/runner/bridge.go`, `internal/runner/delegate.go`, `internal/runner/state.go`, `internal/runner/delegate_test.go` |
| Provider adapters: Codex + Copilot | implemented with deterministic and real smoke evidence | `internal/runner/provider.go`, `internal/runner/process_integration_test.go`, `docs/runner-settings.md` |
| Participant task visibility/controls | implemented | `internal/taskweb/web.go`, `internal/tasks/participant_view.go`, `tests/integration/task_observability_http_test.go` |
| Additive MCP task tools and strict schemas | implemented | `internal/mcpserver/tasks.go`, `tests/integration/tasks_http_test.go` |
| Packaging/docs for machine-local runner ops | implemented | `docs/runner-service.md`, `docs/runner-settings.md`, `docs/tasks.md`, `README.md` |

## Deterministic vs real-model evidence classification

| Scope | Deterministic CI evidence | Real-model / live evidence |
| --- | --- | --- |
| 3.5 Codex↔Copilot continuity and normalized outcomes | `internal/runner/process_integration_test.go` covers `codex→copilot` and `copilot→codex` continuation flows via two independent supervisors and delegate bridge. Provider normalization/permission/interrupt surfaces are covered in `internal/runner/provider_runtime_test.go`, `internal/runner/delegate_test.go`, `internal/runner/supervisor_test.go`. | `docs/runner-settings.md#verified-local-adapter-smoke-commands` records authenticated two-turn continuity for Codex and Copilot using exact thread/session IDs. |
| 5.2 hosted CI + deploy/readiness | Repository gates run in CI and local Go suite; OpenSpec strict validation runs locally. | Railway production endpoint checks return `200` on `/` and `/readyz`, and unauthenticated `/mcp` returns `401` (public auth/readiness contract preserved). `docs/live-deployment.md` records authorized deployment and health behavior. |
| 5.3 physical-machine delivery gate | Local deterministic tests prove bridge/supervisor behavior independent of a specific machine. | Physical-machine evidence is tracked in the archived `architect-led-orchestration` delivery gate (task 10.3) and related operator records for Mac architect + Copilot executor on `192.168.1.223`. |

Claude-specific provider execution/resume evidence is intentionally tracked in
the separate `claude-adapter-followup` change.
