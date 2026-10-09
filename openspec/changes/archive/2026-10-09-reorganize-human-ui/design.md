# Design

## Context

See proposal.md for motivation. The app renders HTML in Go. Home and workspace
administration use `internal/web/templates/pages.html`; backlog and delegated
tasks define separate inline templates. `internal/identity` also has fallback
home/member templates. Agent issuance results and runner enrollment completion
use separate layouts. Current project intake validates browser sessions and
GitHub repository authority through `backlog.Store`, independently of legacy
workspace roles. The home renderer currently lists workspace memberships only.

## Goals / Non-Goals

**Goals:** Make the human entry path discoverable; reuse the existing stores and
permissions; create one page layout and one canonical renderer per feature;
provide safe compatibility for bookmarked URLs and existing forms.

**Non-Goals:** Build a SPA, replace GitHub login, change lifecycle or council
rules, create new projects from the UI, remove supported mailbox capabilities,
or implement the autonomous runner dispatch loop. No broad store refactor or
database record deletion is part of UI cleanup.

## Decisions

### 1. Canonical page structure

| Page | Canonical GET address | Purpose |
| --- | --- | --- |
| Entry | `/` | Anonymous login; authenticated project resolution/chooser |
| Profile | `/profile` | Current GitHub user identity and protected logout |
| Project objectives | `/projects/{project}` | Objective creation, backlog and current statuses |
| Objective | `/projects/{project}/intentions/{id}` | Summary, progress, result, history and creator controls |
| Settings entry | `/settings` | Discover permitted workspace settings, including for users without a project |
| Project settings | `/projects/{project}/settings` | Project context and permitted links to its workspace settings |
| Workspace settings | `/workspaces/{workspaceID}/settings` | Existing members, invitations, owned agents and credentials |
| Legacy delegated tasks | `/workspaces/{workspaceID}/tasks` | Existing participant-owned task list reached from settings |
| Legacy task detail | `/workspaces/{workspaceID}/tasks/{taskID}` | Existing participant-owned instruction/result and controls |
| Invitation acceptance | `/invitations/accept` | Explicit invitation workflow |
| Runner completion | `/runners/enrollment/{id}` | Enrollment completion and return-to-terminal guidance |

Project dashboard links are labeled `Objectives`; workspace agent-to-agent work
is labeled `Delegated tasks` and remains in settings. This keeps distinct access
policies and meanings visible instead of treating every task as a human goal.
Separate top-level dashboards for architects/executors are not introduced.

### 2. Selection and access

List configured active projects through an authorized read service using the
current human session and the same repository authority as backlog intake.
Do not infer project access solely from workspace membership or a public repo.
Bound candidate reads and recheck authority; unavailable checks fail closed.
Persist only a project UUID as the last selection in an owner-session-scoped
preference or browser cookie, treated as an untrusted hint on every request.
No server-side session schema change is required for a cookie hint.

At `/`, select the only accessible project automatically. With several, use a
still-authorized saved selection or render a chooser. With none, render an empty
state and permitted Settings access. A selected project resolves to its canonical
dashboard; `/` and `/projects/{project}` do not contain separate dashboard code.
Switching projects returns to the new project's dashboard rather than carrying
an objective ID or pagination cursor from another project.

Keep current GitHub return-to validation. Ordinary login returns to home; valid
invitation, objective and runner enrollment links retain their intended return
path. Reorganization must not replace an enrollment return with the dashboard.

### 3. Shared shell and interaction

Use shared embedded Go templates/CSS for The Firm branding, authentication
controls, breadcrumbs/back links, project context, and a persistent navigation
bar. Signed-in pages expose a project chooser, `Objectives`, `Settings`,
`Profile`, and protected `Log out`, with the active page identified. Without a
selected authorized project, show project selection rather than a broken
Objectives link. Settings and Profile remain reachable for eligible signed-in
users without project access. On narrow screens the same links remain keyboard
accessible through a wrapping or collapsible navigation layout.

`/profile` derives the current user only from the authenticated browser session
and shows the existing GitHub username and immutable account identifier. It
includes the existing CSRF-protected logout action and links back to available
project work. It does not add profile editing, password settings, or provider
account management. Anonymous profile requests return to the sign-in flow using
the existing validated return mechanism.

Special invitation/enrollment and one-time secret results use the shared shell
without implying authorization to project or admin content. Anonymous variants
show branding and sign-in; authenticated variants retain the navigation bar.

Use visible form labels and focus styles, semantic headings, text status labels,
responsive layouts and consistent error/empty-state components. A status must
be understandable without color. Preserve submitted non-secret objective fields
on validation errors and keep the existing idempotency key semantics. Browser
UI labels remain English as required by CONTRIBUTING.md.

Keep POST endpoints protected by current authorization, same-origin and CSRF.
Follow successful ordinary mutations with the existing POST/redirect/GET flow
to canonical pages. Credential issuance/rotation and invitation creation retain
their protected one-time result pages; do not redirect away before the user has
received the newly created secret, or persist it for redisplay.

### 4. Objective summary and evidence

Build a project-authorized read projection combining existing backlog revisions,
lifecycle history, and recorded orchestration records associated with the same
project/root. Present title/description, friendly status with blocked reason,
recorded progress, available outcome, and chronological history in the primary
view. Use timestamps and existing authoritative ordering for history; preserve
source identifiers for technical inspection.

Contracts, votes, assignments, questions and verification evidence belong in a
secondary technical section using the same page, such as `<details>`. Preserve
existing historical revision read semantics rather than overwriting originals.
When evidence does not exist or runtime dispatch is unavailable, show the known
submitted/blocked state and a clear absence of further progress. Do not infer
execution from machine presence or calculate invented completion percentages.

Project authorization never substitutes for legacy task participant ownership.
Only project-scoped evidence can be joined by project/root. A link to an old
delegated task continues through its existing authorization checks.

### 5. Route and cleanup inventory

| Existing page/route | Planned treatment |
| --- | --- |
| GET `/` workspace list | Replace with login/project entry; preserve workspace discovery under Settings |
| Header username/logout | Keep logout contract; add `/profile` and persistent navigation |
| GET `/projects/{project}/intentions` | Redirect to `/projects/{project}`; preserve validated `after` cursor |
| GET `/projects/{project}/intentions/{id}` | Keep canonical address; replace isolated detail layout |
| GET `/workspaces/{workspaceID}` | Redirect to `/workspaces/{workspaceID}/settings` |
| GET delegated-task list/detail | Keep addresses and permissions; share shell and link from Settings |
| GET `/invitations/accept` | Keep flow, parameters and intended-account protections; share shell |
| GET `/runners/enrollment/{id}` | Keep completion route and enrollment return behavior; share shell |
| POST intake/revision/control routes | Keep endpoints; update form rendering and canonical success destinations |
| POST member/invitation/agent/credential routes | Keep endpoints and semantics; update settings return links |
| GitHub login/callback, POST logout | Keep authentication contracts |
| `/assets/`, `/healthz`, `/readyz`, `/mcp`, machine APIs | Preserve service interfaces |

Use temporary same-origin redirects for superseded authenticated GET pages,
not permanent cached redirects or POST redirects to new mutation targets.
Validate the target resource and copy only supported query parameters, never an
arbitrary `return_to` supplied to a compatibility redirect.

During implementation, produce a file-level cleanup inventory with each file,
template branch or helper, its replacement and reference evidence. Candidates
include obsolete workspace-home branches, duplicated inline shells in backlog
and taskweb, and fallback renderers only if proven unreachable in every supported
configuration. Do not predeclare an entire package dead. Delete a candidate only
after all active routes/configurations use the replacement and tests pass.
Compatibility redirect handlers remain maintained code, not deletion candidates.

### Alternatives

Keeping home as workspace administration leaves the main objective flow hidden.
Renaming every route, including mutations, adds avoidable integration risk.
Deleting legacy tasks or credential pages would remove supported functionality
and change permissions. A new frontend framework adds infrastructure without
being necessary for this navigation and layout change.

## Risks / Trade-offs

- Repository checks can fail or be slow → use bounded authorized reads and
  explicit unavailable states without listing projects on assumed access.
- Mixed project and workspace authority → test collaborators, members, admins,
  participant owners and unrelated users independently.
- Runtime data may be incomplete → show only recorded progress and outcomes;
  runtime completion remains separate work.
- Bookmarks, OAuth return paths and one-time results are easy to break → retain
  endpoint contracts and test those flows before deleting old rendering code.
- Alternate feature configurations may still use fallback pages → prove reachability
  under identity-only, mailbox/task-enabled and runner-identity configurations.

## Migration Plan

1. Inventory routes and renderer references; add canonical pages and shared shell
   using the current data/authorization model.
2. Wire project discovery, entry, dashboard and detail; verify existing forms,
   historical records, access boundaries and empty/error states.
3. Move administration into settings and keep legacy task discovery there.
4. Add GET compatibility redirects and remove proven superseded renderers/links.
5. Run HTTP, real-PostgreSQL and browser-flow checks; document new navigation and
   evidence before archive. Public deployment is a separate authorized action.

No destructive data migration is planned. Rollback restores the previous routes
and templates while keeping all existing records and form endpoints intact.
