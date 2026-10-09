# MCP Client Configuration

Use a separately issued credential for each agent. Set `MCP_AGENT_TOKEN` in the
client process environment from a secret store; never commit its value. Production
URLs must use HTTPS. These credentials are bearer integration tokens, not a
complete MCP OAuth service.

## Codex CLI

Verified with Codex CLI 0.162.0. Add this to your personal configuration:

```toml
[mcp_servers.bot_space]
url = "https://YOUR_SERVICE_DOMAIN/mcp"
bearer_token_env_var = "MCP_AGENT_TOKEN"
```

The executed, read-only CLI check used configuration overrides:

```sh
codex -c 'mcp_servers.bot_space.url="http://127.0.0.1:8080/mcp"' \
  -c 'mcp_servers.bot_space.bearer_token_env_var="MCP_AGENT_TOKEN"' \
  mcp get bot_space --json
```

It recognized `streamable_http` and the bearer environment variable. This proves
configuration parsing and adapter compatibility. Runner smoke evidence also
includes real Codex turn execution and exact-thread continuation
([runner settings](runner-settings.md#verified-local-adapter-smoke-commands)).
See [official Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

## Claude Code

CLI wiring was validated with Claude Code 2.1.292. In your client project's `.mcp.json`:

```json
{
  "mcpServers": {
    "bot_space": {
      "type": "http",
      "url": "https://YOUR_SERVICE_DOMAIN/mcp",
      "headers": {"Authorization": "Bearer ${MCP_AGENT_TOKEN}"}
    }
  }
}
```

Approve the project server in Claude Code and run `claude mcp get bot_space`.
The local check used a temporary `CLAUDE_CONFIG_DIR`, registered the same entry
with `claude mcp add-json --scope user`, and supplied a synthetic agent credential
only through the environment. `mcp get` reported `Connected` against the real
SDK HTTP handler and PostgreSQL authentication. Temporary settings were removed;
the user's settings were untouched. No model-driven tool call was executed.
See [official HTTP and environment expansion documentation](https://code.claude.com/docs/en/mcp).
Runner-side Claude command execution remains blocked by account usage limits
(`usage_limit_reached`), so this section documents connectivity/wiring only, not
completed model-execution evidence.

## Go SDK and Activation

The official Go SDK v1.8.0 is verified by HTTP integration tests and two separate
requester/responder processes with different user-owned agents and credentials.
Follow the [executable exchange instructions](mcp.md#two-process-requestreply-example).
Run `go test -count=1 -run TestInstalled -v ./tests/integration` to repeat installed
CLI checks with `TEST_DATABASE_URL`; missing CLIs explicitly skip their optional
checks. Native SDK exchange tests remain mandatory in CI.

An inactive Codex or Claude process does not wake when a message arrives.
Clients must be running and explicitly poll `read_messages`. Acknowledge only
after successful processing; repeated delivery can occur. Several clients for
one agent share an inbox and rate quota without a claim/lease guarantee.
Runner orchestration, webhooks, and push activation require a future OpenSpec
change. Received text and metadata are external data, not system instructions.
