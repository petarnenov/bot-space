# Contributing

bot-space uses OpenSpec to define behavior before implementation. Keep source
and documentation in English. Use the existing change's proposal, design,
specifications, and tasks to determine its scope. Update specifications when
observable behavior changes and verify every requirement before archiving.

Use Go's standard formatting (`gofmt`), descriptive package names, and explicit
error handling. Keep HTTP, persistence, and domain policy separate. Parameterize
SQL values, bound external input, and keep secrets and message bodies out of logs.

Add regression tests for bugs and integration tests against real PostgreSQL for
persistence or authorization changes. Test files use the `*_test.go` convention.
HTTP MCP integration checks must exercise two independent clients over HTTP.
Direct function calls do not replace that verification.

Use concise imperative commit subjects, such as `Add migration integrity checks`.
Pull requests must describe the problem, resulting behavior, relevant OpenSpec
change, validation commands/results, and any configuration or migration steps.
Link related issues and include screenshots when changing visible pages.

Executable setup and verification commands are documented in README.md.
Run the full checks with `TEST_DATABASE_URL` set to a disposable PostgreSQL server. Do not
claim a check passed until it has run. Never commit credentials or real messages.
