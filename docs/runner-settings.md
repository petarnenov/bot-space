# Runner client settings

The local runner CLI uses an owner-only JSON configuration file and local control commands:

```sh
runner doctor --config /absolute/private/runner.json
runner serve --config /absolute/private/runner.json
runner start-task --config /absolute/private/runner.json --agent AGENT_UUID --instruction "Return a synthetic analysis"
runner status --config /absolute/private/runner.json
```

Use independent state directories and configuration files for architect and executor processes sharing a host. The local supervisor lock prevents duplicate processes from sharing one state directory.

Configuration maps each managed agent to a fixed local provider, project directory, token environment variable, allowed sender IDs and optional model/effort policy. Executor concurrency is fixed at one.

Codex receives `-m` and `model_reasoning_effort`; Claude receives `--model` and `--effort`; Copilot receives `--model` and `--reasoning-effort`. Model availability and supported effort depend on the installed client and account; no automatic model substitution is permitted. Syntax validation and adapter routing are implemented; live model/effort compatibility preflight remains pending.

Verify local configuration/locking/routing behavior with:

```sh
go test -race ./internal/runner ./cmd/runner
```

These tests do not claim model-authenticated execution or public gRPC readiness.

## Managed session continuation

Delegated results are correlated per job and resumed only into the exact stored
provider session for that job. The runner never uses "last session" selectors.

If a provider turn ends before dependent delegated work is delivered, the job is
persisted as `waiting_dependency` and kept in the private journal. On restart,
the supervisor reconciles those dependencies and queues one continuation turn in
that same session, or marks the job `requires_approval` with
`uncertain_interruption` when safe automatic continuation is not provable.

## Verified local adapter smoke commands

Observed on this machine:

- Codex CLI: `codex-cli 0.162.0`
- Claude Code: `2.1.292`
- GitHub Copilot CLI: `1.0.95`

Executed checks:

```sh
# Codex execution
printf 'Return exactly: CODEx_OK\n' | codex exec --json --color never -

# Codex exact resume in the same thread/session
printf 'Remember token: BLUEBANANA. Reply exactly: ACK1\n' | codex exec --json --color never -
printf 'What token did I ask you to remember? Reply exactly with token only.\n' | codex exec resume --json <thread_id_from_turn_1> -

# Copilot execution and exact-session continuity
copilot -p 'Remember token: GREENAPPLE. Reply exactly: ACK1' \
  --session-id 88888888-8888-4888-8888-888888888888 \
  --output-format json --log-level none --no-ask-user --no-auto-update --disable-builtin-mcps
copilot -p 'What token did I ask you to remember? Reply exactly with token only.' \
  --session-id 88888888-8888-4888-8888-888888888888 \
  --output-format json --log-level none --no-ask-user --no-auto-update --disable-builtin-mcps
```

Observed results:

- Codex returned `CODEx_OK`, then resumed the same `thread_id` and returned `BLUEBANANA`.
- Copilot returned `ACK1`, then in the same `sessionId` returned `GREENAPPLE`.
- Claude command execution is currently blocked by account usage limits (`usage_limit_reached`), so real turn execution/continuation evidence is still pending for Claude.
