# Human UI routes

The human interface uses project work as its primary navigation and keeps
workspace administration under Settings. Browser mutations retain their existing
paths so saved forms and authorization checks do not change.

| Route | Purpose | Availability |
| --- | --- | --- |
| `GET /` | GitHub sign-in or authorized project entry | Always with browser identity |
| `GET /projects/{project}` | Objective creation, list and status | Runner identity enabled |
| `GET /projects/{project}/intentions/{id}` | Objective summary, progress and history | Runner identity enabled |
| `GET /profile` | Current GitHub identity and logout | Authenticated users |
| `GET /settings` | Workspace settings index | Authenticated users |
| `GET /projects/{project}/settings` | Settings entry in project context | Authorized project users |
| `GET /workspaces/{workspaceID}/settings` | Members, invitations, agents and credentials | Workspace members |
| `GET /workspaces/{workspaceID}/tasks` | Participant-owned delegated tasks | Task feature enabled |
| `GET /workspaces/{workspaceID}/tasks/{taskID}` | Delegated task details | Task feature enabled |
| `GET /invitations/accept` | Invitation acceptance | Browser identity enabled |
| `GET /runners/enrollment/{id}` | Runner enrollment completion | Runner identity enabled |

Compatibility routes:

- `GET /projects/{project}/intentions` redirects to `/projects/{project}` and
  preserves a valid `after` cursor.
- `GET /workspaces/{workspaceID}` redirects to
  `/workspaces/{workspaceID}/settings` after workspace authorization.

The following renderers are replaced by canonical pages: the workspace-list home
branch in `internal/web/templates/pages.html`, the fallback identity home/member
templates when management is registered, and the standalone backlog list page.
Fallback identity renderers remain for identity-only configurations. Delegated
task, invitation, one-time credential and runner enrollment pages remain active
because they have distinct behavior or access rules; their shared navigation is
updated rather than deleting those routes.

Service routes (`/assets/`, `/healthz`, `/readyz`, `/mcp`, authentication
callbacks and machine APIs) are not human navigation pages and remain unchanged.

## Interaction conventions

- Human pages use one dark, responsive theme and a consistent Objectives,
  Settings, and Profile navigation bar.
- The navigation includes a project drop-down populated only with repositories
  the signed-in GitHub user is currently authorized to work on. The current
  project is selected, and submitting the switcher opens the chosen project.
- The selected project is validated against current GitHub repository authority
  before it is saved or opened.
- Objective descriptions use the primary full-width editor (18 visible rows,
  vertically resizable) and accept up to 32,768 characters.
- Objective submissions keep the existing CSRF, same-origin, idempotency, and
  project-authority checks. Invalid fields retain non-secret input; an expired
  form tells the user to reload rather than returning an unexplained Forbidden.
- Objective details separate the human summary, status, outcome, and history
  from recorded technical counts for decisions, assignments, and attempts.
