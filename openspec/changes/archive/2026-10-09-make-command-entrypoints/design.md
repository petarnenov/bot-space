# Design

## Scope

This is a tooling/documentation normalization change. It does not alter protocol
schemas, runtime feature contracts, authorization, storage, or deployment model.

## Make entrypoint strategy

- Provide explicit phony targets for:
  - local compose flows (`compose-up/down/logs/recreate-app`)
  - mailbox commands (`migrate`, `serve`)
  - verification (`fmt-check`, `mod-verify`, `vet`, `test`, `test-race`,
    `build`, `vulncheck`, aggregate `verify`)
  - OpenSpec helpers (`openspec-list/status/apply/archive/validate-*`)
  - Railway helpers (`railway-status`, `railway-deploy`, `railway-healthcheck`)
  - local integration DB lifecycle (`test-db-up`, `test-db-down`)
- Keep each target as a direct wrapper over already-validated commands.
- Support argumentized targets through variables (`CHANGE`, `PKG`, `MESSAGE`,
  `SERVICE`, `ENVIRONMENT`, `PUBLIC_URL`).

## Documentation updates

- Replace direct command sequences in `README.md` with `make` invocations where
  equivalent targets exist.
- Keep non-`make` prerequisites and explanatory text intact.
