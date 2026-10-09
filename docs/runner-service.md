# Runner packaging and service operations

For foreground identity mode, run `make runner-executor`
from the repository root. It builds the binary and starts the executor against
the production server. Use `make runner-architect`
for the architect role.
Each role uses its own private `$HOME/.bot-space/<role>` state directory,
automatically created by the runner, independently of project scopes. The current
CLI requires at least one project. For now, `RUNNER_PROJECTS` defaults to the
current project `bb25680f-eeea-4cde-b229-ddec09961c73`; override it with a
space-separated list for other project scopes. Override
`RUNNER_SERVER` and `RUNNER_STATE` as needed. Follow the printed sign-in URL when prompted.

Build the runner binary from a clean checkout:

```sh
go build -o bin/runner ./cmd/runner
```

Use one private state directory per role and keep it outside project source.
All runner configuration files must be owner-only (`chmod 600`).
Run exactly one supervisor process per physical machine role.

## Configuration examples

The same JSON format applies on macOS and Linux:

```json
{
  "endpoint": "https://YOUR_SERVICE_DOMAIN/mcp",
  "state_dir": "/absolute/private/runner-executor",
  "concurrency": 1,
  "agents": [
    {
      "agent_id": "11111111-1111-4111-8111-111111111111",
      "token_env": "BOTSPACE_EXECUTOR_TOKEN",
      "provider": "copilot",
      "executable": "copilot",
      "project": "/absolute/path/to/local/checkout",
      "model": "gpt-5-mini",
      "effort": "medium",
      "policy": "workspace-write",
      "allowed_senders": [
        "22222222-2222-4222-8222-222222222222"
      ]
    }
  ]
}
```

Store this as an owner-only file, for example:

```sh
install -m 600 /tmp/runner.json /absolute/private/runner.json
```

## Foreground operation

```sh
# Local supervised runtime
./bin/runner serve --config /absolute/private/runner.json

# Local health/config checks
./bin/runner doctor --config /absolute/private/runner.json
./bin/runner status --config /absolute/private/runner.json
```

Use identity enrollment/refresh mode only with explicit server/role/project flags:

```sh
./bin/runner enroll --server https://YOUR_HOST --state /private/executor --role executor --project PROJECT_UUID --no-open
```

## macOS auto-start (launchd)

Create `~/Library/LaunchAgents/com.botspace.runner.executor.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.botspace.runner.executor</string>
  <key>ProgramArguments</key>
  <array>
    <string>/absolute/path/bin/runner</string>
    <string>serve</string>
    <string>--config</string>
    <string>/absolute/private/runner.json</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/absolute/private/runner.stdout.log</string>
  <key>StandardErrorPath</key><string>/absolute/private/runner.stderr.log</string>
</dict>
</plist>
```

Install / uninstall:

```sh
launchctl load -w ~/Library/LaunchAgents/com.botspace.runner.executor.plist
launchctl unload -w ~/Library/LaunchAgents/com.botspace.runner.executor.plist
```

## Linux auto-start (systemd user service)

Create `~/.config/systemd/user/botspace-runner-executor.service`:

```ini
[Unit]
Description=bot-space runner (executor)
After=network-online.target

[Service]
Type=simple
ExecStart=/absolute/path/bin/runner serve --config /absolute/private/runner.json
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

Install / uninstall:

```sh
systemctl --user daemon-reload
systemctl --user enable --now botspace-runner-executor.service
systemctl --user disable --now botspace-runner-executor.service
```
