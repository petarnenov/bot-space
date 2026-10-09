.PHONY: \
	help \
	compose-up compose-down compose-logs compose-recreate-app \
	test-db-up test-db-down \
	migrate serve \
	fmt-check mod-verify vet test test-package test-race test-race-package build vulncheck \
	openspec-validate-all openspec-validate-change openspec-list openspec-status openspec-apply openspec-archive \
	claude-smoke \
	verify \
	railway-status railway-deploy railway-healthcheck

OPEN_SPEC_TELEMETRY ?= 0
CHANGE ?=
MESSAGE ?= deploy via make
SERVICE ?= bot-space
ENVIRONMENT ?= production
PUBLIC_URL ?= https://bot-space-production.up.railway.app
TEST_DB_CONTAINER ?= botspace-test-pg
TEST_DB_PORT ?= 55432

help:
	@echo "Common targets:"
	@echo "  make compose-up                 # docker compose up --build -d"
	@echo "  make compose-down               # docker compose down"
	@echo "  make test-db-up                # local postgres on $(TEST_DB_PORT) for TEST_DATABASE_URL"
	@echo "  make test-db-down              # stop/remove local test postgres"
	@echo "  make migrate                    # go run ./cmd/mailbox migrate"
	@echo "  make serve                      # go run ./cmd/mailbox serve"
	@echo "  make verify                     # full local verification gates"
	@echo "  make openspec-list              # openspec list --json"
	@echo "  make openspec-status CHANGE=<id>"
	@echo "  make openspec-apply CHANGE=<id>"
	@echo "  make openspec-archive CHANGE=<id>"
	@echo "  make claude-smoke               # real Claude turn probe (expected to fail on quota)"
	@echo "  make railway-status             # linked Railway status"
	@echo "  make railway-deploy MESSAGE='...'"

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down

compose-logs:
	docker compose logs app migrate

compose-recreate-app:
	docker compose up -d --force-recreate app

test-db-up:
	docker rm -f "$(TEST_DB_CONTAINER)" >/dev/null 2>&1 || true
	docker run -d --name "$(TEST_DB_CONTAINER)" -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=mailbox -p "$(TEST_DB_PORT):5432" postgres:16-alpine
	for i in $$(seq 1 30); do docker exec "$(TEST_DB_CONTAINER)" pg_isready -U postgres -d mailbox >/dev/null 2>&1 && exit 0; sleep 1; done; exit 1

test-db-down:
	docker rm -f "$(TEST_DB_CONTAINER)" >/dev/null 2>&1 || true

migrate:
	go run ./cmd/mailbox migrate

serve:
	go run ./cmd/mailbox serve

fmt-check:
	test -z "$$(gofmt -l cmd internal migrations tests examples)"

mod-verify:
	go mod verify

vet:
	go vet ./...

test:
	go test -count=1 ./...

test-package:
	@test -n "$(PKG)" || (echo "PKG is required (example: make test-package PKG=./internal/runner)" && exit 1)
	go test -count=1 "$(PKG)"

test-race:
	go test -race -count=1 ./...

test-race-package:
	@test -n "$(PKG)" || (echo "PKG is required (example: make test-race-package PKG=./internal/runner)" && exit 1)
	go test -race -count=1 "$(PKG)"

build:
	go build ./...

vulncheck:
	go -C cmd/mailbox run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -scan=module

openspec-validate-all:
	OPENSPEC_TELEMETRY=$(OPEN_SPEC_TELEMETRY) openspec validate --all --strict --no-interactive

openspec-validate-change:
	@test -n "$(CHANGE)" || (echo "CHANGE is required (example: make openspec-validate-change CHANGE=claude-adapter-followup)" && exit 1)
	OPENSPEC_TELEMETRY=$(OPEN_SPEC_TELEMETRY) openspec validate --changes "$(CHANGE)" --strict

openspec-list:
	openspec list --json

openspec-status:
	@test -n "$(CHANGE)" || (echo "CHANGE is required (example: make openspec-status CHANGE=claude-adapter-followup)" && exit 1)
	openspec status --change "$(CHANGE)" --json

openspec-apply:
	@test -n "$(CHANGE)" || (echo "CHANGE is required (example: make openspec-apply CHANGE=claude-adapter-followup)" && exit 1)
	openspec instructions apply --change "$(CHANGE)" --json

openspec-archive:
	@test -n "$(CHANGE)" || (echo "CHANGE is required (example: make openspec-archive CHANGE=distributed-agent-runner)" && exit 1)
	openspec archive "$(CHANGE)" --yes --json

claude-smoke:
	printf 'Return exactly: CLAUDE_OK\n' | claude -p --output-format stream-json --verbose --permission-mode dontAsk

verify: fmt-check mod-verify vet test test-race build vulncheck openspec-validate-all

railway-status:
	railway status

railway-deploy:
	railway up --service "$(SERVICE)" --environment "$(ENVIRONMENT)" --ci -m "$(MESSAGE)"

railway-healthcheck:
	curl --fail "$(PUBLIC_URL)/"
	curl --fail "$(PUBLIC_URL)/readyz"
	curl -sS -o /dev/null -w '%{http_code}\n' "$(PUBLIC_URL)/mcp"
