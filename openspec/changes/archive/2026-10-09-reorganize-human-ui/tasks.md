# Tasks

## 1. Route and rendering inventory

- [x] 1.1 Produce a route/renderer cleanup inventory covering identity-only, mailbox/task-enabled, and runner-identity configurations; verify each planned replacement against registered handlers, templates, callers, and the design's compatibility table.

## 2. Project entry and selection

- [x] 2.1 Implement bounded human-authorized project discovery and selected-project validation using existing repository authority; add real-PG/HTTP tests for one/many/no projects, revoked access, unavailable authority, and workspace-role independence, then run the focused integration suite with `TEST_DATABASE_URL`.
- [x] 2.2 Wire `/` and `/projects/{project}` to login, chooser, or the canonical objective dashboard, preserve valid OAuth invitation/enrollment return paths, and document entry behavior; verify HTTP tests for automatic selection, saved selection, project switching, and supported feature configurations.

## 3. Navigation, profile, and intake interaction

- [x] 3.1 Add the shared navigation bar with project context, Objectives, Settings, Profile, active page, logout, and back links across human pages; add responsive/focus/label styling and template tests, document navigation, and verify authenticated/anonymous and no-project states.
- [x] 3.2 Implement `/profile` from the current human session with GitHub identity and protected logout; add HTTP tests for own-profile visibility, anonymous requests, foreign identifiers, secret absence, and access without projects, then run the focused identity/web tests.
- [x] 3.3 Consolidate objective creation/list rendering into the dashboard with consistent statuses, empty/error states, and retained non-secret input on validation failure; preserve idempotency and CSRF, document submission, and verify real-PG/HTTP create/retry/invalid-input cases.

## 4. Objective details and recorded progress

- [x] 4.1 Add a project-authorized read projection for existing revisions, lifecycle history, recorded workflow state, evidence, and available outcomes; verify seeded real-PG tests for chronological history, blocked/completed/no-evidence states, cross-project denial, and legacy participant privacy.
- [x] 4.2 Render one canonical detail page with primary summary/result/history and secondary technical evidence, retaining creator controls and historical reads; update navigation docs and verify HTTP tests for revision/lifecycle concurrency, escaping, missing evidence, preserved history, and absence of approval/override controls.

## 5. Settings and retained legacy flows

- [x] 5.1 Move workspace administration into `/settings`, project settings links, and workspace settings, retaining member/invitation/agent/credential operations and one-time result pages; update settings docs and verify existing role, intended-invitee, credential-ownership, CSRF, and one-time-secret integration tests.
- [x] 5.2 Apply shared navigation to retained delegated-task, invitation, enrollment, and result pages and link permitted delegated tasks from Settings; verify participant ownership, no secret redisplay, valid OAuth return paths, and backward navigation with focused HTTP tests.

## 6. Compatible cleanup

- [x] 6.1 Add authorized same-origin GET redirects for superseded backlog/workspace pages, preserving supported cursors and unchanged POST endpoints; update documented address mappings and verify bookmarks, denied access, invalid query values, and existing form destinations in HTTP tests.
- [x] 6.2 Remove only superseded page renderers, template branches, styles, and links proven unused in the inventory after replacements pass; record reference evidence per deletion and verify supported configurations, route reachability, template execution, and `git diff --check` without data/schema deletion.

## 7. Integrated acceptance

- [x] 7.1 Run `make verify` with `TEST_DATABASE_URL` pointing to a disposable PostgreSQL database, record gate results, and verify the change with `openspec validate reorganize-human-ui --strict`; confirm regression coverage preserves existing records, revisions, audit history, access boundaries, and browser protection headers.
- [ ] 7.2 Complete a browser walkthrough from home through sign-in/project selection, submission, detail/progress/history, Settings, Profile, and return navigation, including narrow viewport and keyboard use; record screenshots/results and prove no retained feature requires a guessed address, with any blocked acceptance check left incomplete.

## Requirement-to-check traceability

| Requirement | Tasks / acceptance evidence |
| --- | --- |
| Authenticated navigation and member administration | 2.1, 3.1, 5.1; role/access and CSRF integration tests |
| Owned agents and one-time credential pages | 5.1, 5.2; ownership and one-time-secret regression tests |
| Project-first landing | 2.1, 2.2, 3.3, 7.2; home-to-submission walkthrough |
| Discoverable contextual navigation | 3.1, 5.2, 6.1, 7.2; reachable links and retained project context |
| Current-user profile | 3.2, 7.2; own-session profile and logout tests |
| Compatible page consolidation | 1.1, 6.1, 6.2, 7.1; redirect and cleanup inventory evidence |
| Consistent accessible interaction | 3.1, 3.3, 7.2; validation, keyboard and narrow viewport checks |
| Human intake and autonomous status | 3.3, 4.1, 4.2; truthful recorded-state fixtures |
| Unified objective detail | 4.1, 4.2, 7.2; summary/result/history and authorization tests |

## Workflow follow-up

- Review the final change and archive only after all tracked verification passes.
- Sync delta specs into main specs during archive; preserve verification and cleanup evidence.
- Commit/push or deploy only when requested; deployment is outside this proposal.
