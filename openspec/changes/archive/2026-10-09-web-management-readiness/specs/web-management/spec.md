# Web Management

## Purpose

Provide minimal usable server-rendered management for private workspace membership and owned agents without exposing mailbox contents.

## ADDED Requirements

### Requirement: Authenticated navigation and member administration
The browser UI SHALL provide GitHub login/logout, workspace selection, member/role views, and authorized role/removal forms. Only current owner/admin SHALL see applicable administrative controls; owners alone SHALL see owner grants. Every mutation SHALL independently enforce server-side authorization and CSRF regardless of visible controls.

#### Scenario: Role-aware administration
- **GIVEN** owner, admin and member sessions
- **WHEN** workspace pages are viewed and protected forms submitted
- **THEN** controls reflect roles and forged/unauthorized submissions are rejected without mutation.

### Requirement: Invitation management and identity-bound acceptance
Owner/admin pages SHALL create, list and cancel targeted member/admin invitations, displaying the secret/link only on creation. An invitation acceptance page SHALL support login and explicit CSRF-protected POST acceptance by the intended GitHub account; forwarding the link SHALL not grant another account membership. Expired/cancelled/consumed invitations SHALL fail safely.

#### Scenario: Invitation link lifecycle
- **GIVEN** an owner-created invitation link for another account
- **WHEN** it is opened, the intended account signs in and accepts, or another account attempts acceptance
- **THEN** only intended acceptance grants membership and listing never redisplays the secret.

### Requirement: Owned agents and one-time credential pages
Workspace pages SHALL list the caller's agents/credential metadata and provide registration, deactivation, issue, revoke and rotation forms. Owner/admin SHALL have authorized team-agent deactivation without credential-issuance privileges over others. Newly issued tokens SHALL appear once on a no-store result page; later views SHALL show metadata only.

#### Scenario: Owned credential workflow
- **GIVEN** a member and another member's agent
- **WHEN** it registers/credentials its own agent or tries another owner's credential operation
- **THEN** own actions succeed, foreign operations fail, and only issuance/rotation shows a raw token.

### Requirement: Private escaped browser rendering
Management responses SHALL escape untrusted names, use no-store/no-referrer and browser protections, and never show inbox content or message bodies to human administrators. The UI SHALL be served by Go with minimal embedded static assets and no separate frontend infrastructure.

#### Scenario: Administrative privacy and escaping
- **GIVEN** a workspace with private messages and an untrusted display label
- **WHEN** an administrator views management pages
- **THEN** no message content appears and the label cannot execute HTML/script.
