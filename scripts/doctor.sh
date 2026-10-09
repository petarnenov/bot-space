#!/bin/sh
# Check tools without installing anything or contacting the production server.
set -eu

cd "$(dirname "$0")/.."
required_go=$(awk '$1 == "go" { print $2; exit }' go.mod)
failed=0

go_help() {
    printf '  Install Go %s or newer: https://go.dev/doc/install\n' "$required_go"
    printf '  On macOS with Homebrew: brew install go (or brew upgrade go).\n'
    printf '  Then open a new terminal and run make setup.\n'
}

if command -v go >/dev/null 2>&1; then
    # A diagnostic must not silently download a toolchain.
    if installed_go=$(GOTOOLCHAIN=local go version 2>/dev/null); then
        installed_go=$(printf '%s\n' "$installed_go" | awk '{ sub(/^go/, "", $3); print $3 }')
        if awk -v installed="$installed_go" -v required="$required_go" 'BEGIN {
            split(installed, have, "."); split(required, need, ".");
            for (i = 1; i <= 3; i++) {
                if (have[i] !~ /^[0-9]+$/) exit 1;
                if (have[i] + 0 > need[i] + 0) exit 0;
                if (have[i] + 0 < need[i] + 0) exit 1;
            }
            exit 0;
        }'; then
            printf 'OK Go %s (required: %s or newer)\n' "$installed_go" "$required_go"
        else
            printf 'FAIL Go %s is unsupported (required: %s or newer)\n' "$installed_go" "$required_go"
            go_help
            failed=1
        fi
    else
        printf 'FAIL Go is on PATH but cannot run.\n'
        go_help
        failed=1
    fi
else
    printf 'FAIL Go is missing.\n'
    go_help
    failed=1
fi

if [ "${1:-}" = '--required-only' ]; then
    exit "$failed"
fi

printf '\nTools for local development (optional for make executor / make architect):\n'
if command -v docker >/dev/null 2>&1; then
    if docker compose version >/dev/null 2>&1; then
        printf 'OK Docker Compose\n'
    else
        printf 'WARN Docker Compose is missing: https://docs.docker.com/compose/install/\n'
    fi
    if docker info >/dev/null 2>&1; then
        printf 'OK Docker Engine is running\n'
    else
        printf 'WARN Docker Engine is unavailable. Start Docker Desktop or your Docker service.\n'
    fi
else
    printf 'WARN Docker is missing: https://docs.docker.com/get-started/get-docker/\n'
    printf '  On macOS with Homebrew: brew install --cask docker; then start Docker Desktop.\n'
fi

if command -v node >/dev/null 2>&1 && node --version >/dev/null 2>&1; then
    printf 'OK Node.js: %s\n' "$(node --version)"
else
    printf 'WARN Node.js is missing or cannot run. Install project target 24.21.0: https://nodejs.org/en/download\n'
fi
if command -v npm >/dev/null 2>&1 && npm --version >/dev/null 2>&1; then
    printf 'OK npm: %s\n' "$(npm --version)"
else
    printf 'WARN npm is missing or cannot run. Install it with Node.js: https://nodejs.org/en/download\n'
fi
if command -v openspec >/dev/null 2>&1 && openspec --version >/dev/null 2>&1; then
    printf 'OK OpenSpec: %s\n' "$(openspec --version)"
else
    printf 'WARN OpenSpec is missing or cannot run. After installing Node.js/npm, run:\n'
    printf '  npm install -g @fission-ai/openspec@1.14.1\n'
fi

printf '\nRunner provider tools (install only those used by your agents):\n'
for provider in codex claude copilot; do
    if command -v "$provider" >/dev/null 2>&1; then
        printf 'OK %s is on PATH (login is checked by runner doctor --config).\n' "$provider"
    else
        printf 'WARN %s is missing. Installation links: README.md, Local tools and setup.\n' "$provider"
    fi
done
printf '\nGo dependencies and binaries: make setup\n'
exit "$failed"
