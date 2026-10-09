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
  It boots two independent supervisors, exercises local bridge delegation from A
  to B, and verifies continuation text includes B's actual returned result.
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
| Provider adapters: Codex + Copilot | implemented with real smoke evidence | `internal/runner/provider.go`, `docs/runner-settings.md`, local command evidence |
| Provider adapter: Claude | blocked by account usage quota | CLI returns `usage_limit_reached` |
| Participant task visibility/controls | implemented | `internal/taskweb/web.go`, `internal/tasks/participant_view.go`, `tests/integration/task_observability_http_test.go` |
| Additive MCP task tools and strict schemas | implemented | `internal/mcpserver/tasks.go`, `tests/integration/tasks_http_test.go` |
| Packaging/docs for machine-local runner ops | implemented | `docs/runner-service.md`, `docs/runner-settings.md`, `docs/tasks.md`, `README.md` |

## Known external limits

1. **Claude provider evidence gate**: real start/resume continuation checks are
   currently blocked by CLI account quota (`usage_limit_reached`), so tasks 3.3
   and Claude-dependent portions of 3.5 remain incomplete.
2. **Hosted publish/deploy gates**: 5.2 requires a reviewed commit, green hosted
   CI for that exact commit, and authorized Railway verification.
3. **Two-physical-machine proof**: 5.3 requires two accessible authenticated
   physical machines with runner/provider setup; this is environment dependent.

Until those external conditions are satisfied, synchronization/archive for this
change must remain pending.
