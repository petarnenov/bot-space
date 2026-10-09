# Proposal

## Why

The current home page emphasizes workspace administration while human objectives
are reachable through separate project URLs. Users need a discoverable path from
sign-in to submitting work, following progress, and reading results, with one
coherent navigation structure and no duplicate legacy screens.

## What Changes

- Make `/` the GitHub sign-in entry for anonymous users and the entry to the
  selected project's objective dashboard for authenticated users.
- Provide a persistent navigation bar with authorized project selection,
  Objectives, Settings, Profile, logout, explicit current-project context, and
  contextual back links.
- Add a profile page for the current GitHub user's identity and session controls.
- Combine objective creation and the existing backlog in one project dashboard.
- Present description, actual workflow status, progress, outcome, and history in
  one objective detail page; place technical evidence in a secondary section.
- Move workspace membership, invitations, owned agents, and credential controls
  into settings, preserving existing authorization and one-time secret behavior.
- Consolidate duplicate page renderers and templates. Redirect superseded GET
  addresses to equivalent canonical pages; keep active mutation endpoints and
  authentication/enrollment callbacks compatible.
- Apply consistent English product labels, The Firm branding, forms, errors,
  empty states, responsive layout, and keyboard navigation.

## Capabilities

### New Capabilities

None; the change reorganizes existing human-facing capabilities.

### Modified Capabilities

- `web-management`: Project-first landing, navigation, separated administration,
  current-user profile, consistent interaction, and safe retirement of duplicate pages.
- `project-observability`: A cohesive human objective page with truthful status,
  progress, results, history, and secondary technical detail.

## Impact

Affected areas include `internal/identity`, `internal/web`, `internal/backlog`,
`internal/taskweb`, `internal/agents`, server route registration, project-authority
queries, embedded templates/CSS, HTTP/integration tests, and navigation docs.
No separate frontend infrastructure or new authentication system is introduced.
Existing records, revisions, audit history, access boundaries, and legacy mailbox
workflows are preserved. Legacy delegated-task pages remain accessible through
settings under their participant-ownership policy.

Scope is presentation, authorized reads, navigation, and page consolidation.
This change does not implement the missing autonomous architect execution loop,
new approval controls, project provisioning, a new task lifecycle, or deployment.
Missing runtime evidence is displayed honestly; runtime orchestration remains
separate work. Cleanup removes only page code proven superseded after its
replacement is verified, not domain stores, migrations, or stored history.
