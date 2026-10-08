#!/usr/bin/env python3
"""Exercise local Compose startup, replacement, restart, and database outage.

Uses synthetic data in a temporary fixture table. Run only on the local
development Compose stack, with .env configured. Python 3 is required.
"""

import json
import os
import subprocess
import time
import urllib.error
import urllib.request


def compose(*args, **kwargs):
    return subprocess.run(["docker", "compose", *args], check=True, **kwargs)


def sql(statement):
    result = compose(
        "exec", "-T", "db", "sh", "-c",
        'exec psql -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "$POSTGRES_DB"',
        input=statement, text=True, capture_output=True,
    )
    return result.stdout.strip()


base = "http://127.0.0.1:" + os.environ.get("SMOKE_PORT", "8080")


def status(path):
    try:
        with urllib.request.urlopen(base + path, timeout=3) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def ready():
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        try:
            code, body = status("/readyz")
            if code == 200 and json.loads(body)["status"] == "ready":
                return
        except (OSError, ValueError):
            pass
        time.sleep(0.25)
    raise RuntimeError("local Compose readiness deadline exceeded")


def fixture():
    assert sql("SELECT count(*) FROM mailbox.foundation_smoke_fixture WHERE value='synthetic-persistence-fixture';") == "1"


compose("up", "-d")
ready()
assert status("/healthz")[0] == 200
assert status("/mcp")[0] == 404
assert status("/members")[0] == 404
sql("CREATE TABLE IF NOT EXISTS mailbox.foundation_smoke_fixture (value text PRIMARY KEY); INSERT INTO mailbox.foundation_smoke_fixture VALUES ('synthetic-persistence-fixture') ON CONFLICT DO NOTHING;")
fixture()

compose("up", "-d", "--no-deps", "--force-recreate", "app")
ready()
fixture()
print("Application replacement preserves committed PostgreSQL state.")

compose("down")
compose("up", "-d")
ready()
fixture()
print("Ordinary Compose down/up preserves the named database volume.")

compose("stop", "db")
try:
    start = time.monotonic()
    assert status("/readyz")[0] == 503
    assert time.monotonic() - start < 3
    assert status("/healthz")[0] == 200
finally:
    compose("start", "db")
ready()
fixture()
sql("DROP TABLE mailbox.foundation_smoke_fixture;")
print("Database outage changes readiness without failing liveness; recovery succeeds.")
