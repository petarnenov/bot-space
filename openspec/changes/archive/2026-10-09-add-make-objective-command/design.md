# Design

## Context

See proposal.md. Browser intake authenticates through a server-side session before the backlog store writes an objective. A local Make target cannot safely recover an HttpOnly browser cookie, so operator intake needs an explicit trusted path that still resolves a real human and verifies repository authority.

## Goals / Non-Goals

**Goals:**

- Provide a single reproducible Make command for direct queue intake.
- Reuse backlog normalization, idempotency, revision, and audit behavior.
- Preserve human creator identity and current repository authorization.

**Non-Goals:**

- Create anonymous, runner-originated, or unaudited root objectives.
- Add a public administrative HTTP endpoint or long-lived intake token.
- Change browser intake or queue execution.

## Decisions

Add `mailbox objective` as an operator command and expose it through `make objective`. The command uses `DATABASE_URL` and the configured GitHub App authority, resolves `--github-user-id` to an existing user, verifies the requested project, and calls a shared backlog creation transaction without manufacturing a browser session.

Require `PROJECT`, `GITHUB_USER_ID`, and `TITLE`. Accept exactly one of `DESCRIPTION` or `DESCRIPTION_FILE`; the file form supports large multiline assignments without unsafe shell interpolation. `PRIORITY` defaults to 2, while `TICKET` and `KEY` are optional. An omitted key is generated for a single new submission; an explicit key enables safe retries.

Print only the Objective ID and project ID as structured log fields. Errors remain sanitized and must not print the description, database URL, GitHub App key, or other secrets.

## Risks / Trade-offs

- [Direct database access is privileged] → The command requires operator configuration and still verifies a real human's current repository authority.
- [Shell quoting can alter large input] → `DESCRIPTION_FILE` reads the maintained content directly and is mutually exclusive with inline `DESCRIPTION`.
- [Retries can duplicate work without a stable key] → Document `KEY` for retried automation and preserve existing idempotency conflict behavior.
