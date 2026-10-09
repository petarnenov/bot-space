## MODIFIED Requirements

### Requirement: Authenticated navigation and member administration
The browser UI SHALL provide GitHub login/logout, authorized project selection, workspace selection in settings, member/role views, and authorized role/removal forms. Administration SHALL be separate from objective submission and tracking. Only current owner/admin SHALL see applicable administrative controls; owners alone SHALL see owner grants. Every mutation SHALL independently enforce server-side authorization and CSRF regardless of visible controls.

#### Scenario: Role-aware administration
- **GIVEN** owner, admin and member sessions
- **WHEN** workspace settings are viewed and protected forms submitted
- **THEN** controls reflect roles and forged/unauthorized submissions are rejected without mutation.

#### Scenario: Project access is not workspace administration
- **GIVEN** a GitHub collaborator with project access but no workspace administrative role
- **WHEN** the user opens the project dashboard or settings
- **THEN** permitted project work remains accessible without granting workspace administration or another user's credentials.

### Requirement: Owned agents and one-time credential pages
Workspace settings SHALL list the caller's agents/credential metadata and provide registration, deactivation, issue, revoke and rotation forms. Owner/admin SHALL have authorized team-agent deactivation without credential-issuance privileges over others. Newly issued tokens SHALL appear once on a no-store result page with a return link to settings; later views SHALL show metadata only.

#### Scenario: Owned credential workflow
- **GIVEN** a member and another member's agent
- **WHEN** it registers/credentials its own agent or tries another owner's credential operation
- **THEN** own actions succeed, foreign operations fail, and only issuance/rotation shows a raw token.

## ADDED Requirements

### Requirement: Project-first landing
Anonymous visitors to the home page SHALL see GitHub sign-in. Authenticated users SHALL reach the selected authorized project's objective list, creation form, and current statuses from home. With one accessible project it SHALL be selected automatically; with several and no valid selection, home SHALL offer project selection. With no accessible projects, home SHALL show a clear empty state without requiring a guessed URL.

#### Scenario: Anonymous entry
- **GIVEN** a visitor without a current browser session
- **WHEN** the visitor opens `/`
- **THEN** GitHub sign-in is visible and no private project content is rendered.

#### Scenario: Submit from home
- **GIVEN** an authenticated user with one authorized project
- **WHEN** the user opens home, completes the objective form, and submits it
- **THEN** the existing intake creates one objective and opens its detail page, reachable again from the dashboard.

#### Scenario: Several projects
- **GIVEN** an authenticated user with several authorized projects and no valid selection
- **WHEN** the user opens home
- **THEN** a project chooser leads to the selected project's objective dashboard.

#### Scenario: Revoked or absent project access
- **GIVEN** a stale project selection or no currently accessible projects
- **WHEN** home or a selected-project address is requested
- **THEN** unauthorized content is withheld and a safe chooser, empty state, or access error replaces the stale view.

### Requirement: Discoverable contextual navigation
Authenticated human pages SHALL share a persistent navigation bar with product branding, project selection when available, Objectives, Settings, Profile, and logout. It SHALL identify the current project and active page, with contextual back links. Links SHALL respect current access and feature availability. Every retained feature SHALL be reachable through authorized navigation or its explicit invitation/enrollment flow without knowing an internal URL.

#### Scenario: Follow work without an address
- **GIVEN** a signed-in user on the selected project dashboard
- **WHEN** the user opens an objective and returns to Objectives
- **THEN** the correct project context and objective list are retained without visiting administration.

#### Scenario: Legacy features remain discoverable
- **GIVEN** a user authorized for workspace settings and participant-owned delegated tasks
- **WHEN** the user follows Settings navigation
- **THEN** membership, permitted agent controls, and delegated-task pages can be reached without guessing addresses.

#### Scenario: Profile available throughout navigation
- **GIVEN** an authenticated user on a dashboard, objective, settings, or result page
- **WHEN** the user follows Profile in the navigation bar
- **THEN** the current user's profile opens with the active location indicated and a route back to available project work.

### Requirement: Current-user profile
The Profile page SHALL display the current authenticated human's GitHub identity and provide logout through the existing protected browser action. It SHALL be reachable from the shared navigation, including when the user has no accessible projects. It SHALL not display another user's profile through supplied identifiers or expose browser session secrets, agent credentials, or provider tokens.

#### Scenario: Read own profile
- **GIVEN** a current authenticated GitHub session
- **WHEN** the user opens Profile
- **THEN** the user's GitHub display identity and account identifier are shown with a protected logout control.

#### Scenario: Profile without project access
- **GIVEN** an authenticated user with no accessible projects
- **WHEN** the user opens home and follows Profile
- **THEN** the user's own profile remains available without granting any project or workspace access.

#### Scenario: Anonymous or foreign profile request
- **GIVEN** an anonymous visitor or a request supplying another user's identifier
- **WHEN** Profile is requested
- **THEN** no foreign profile or secret is disclosed and authentication is required for current-user content.

### Requirement: Compatible page consolidation
Superseded read-only page addresses SHALL redirect to their equivalent canonical pages while preserving resource identity and supported pagination context. Unrelated or unauthorized content SHALL remain inaccessible. Active browser mutation endpoints, login callbacks, and enrollment completion flows SHALL remain compatible. Cleanup SHALL remove only proven superseded page code and links while preserving stored records, revisions, and audit history.

#### Scenario: Bookmarked legacy backlog
- **GIVEN** an authorized user with a bookmarked project backlog page and pagination cursor
- **WHEN** the user opens that address
- **THEN** it reaches the equivalent project dashboard with the same project and supported cursor, without a duplicate backlog renderer.

#### Scenario: Existing mutation and history
- **GIVEN** an existing form endpoint, objective revision, invitation, or owned credential
- **WHEN** page consolidation is delivered
- **THEN** authorized actions retain their protection and semantics, historical records remain readable, and one-time secrets are not redisplayed.

### Requirement: Consistent accessible interaction
Pages SHALL use consistent labels, status vocabulary, form styling, error messages, and empty states. Navigation and forms SHALL work by keyboard with visible focus and labeled controls. Narrow screens SHALL retain readable content and usable actions. Invalid objective input SHALL show actionable validation and preserve entered fields without mutating data; authorization failures SHALL not disclose protected content.

#### Scenario: Invalid objective input
- **GIVEN** an authorized user entering an invalid objective
- **WHEN** submission is rejected
- **THEN** the form retains non-secret input and explains the invalid fields without creating an objective.

#### Scenario: Keyboard and narrow-screen use
- **GIVEN** a user operating by keyboard or using a narrow viewport
- **WHEN** the user selects a project, submits an objective, and opens its details
- **THEN** labeled controls, visible focus, understandable statuses, and usable navigation remain available.
