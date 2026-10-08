# Workspace Invitations

## Purpose

Allow authorized administrators to invite a specific GitHub account into a team without enabling public self-enrollment.

## ADDED Requirements

### Requirement: Targeted invitation issuance
Only workspace owners/admins SHALL issue invitations for a positive immutable GitHub user ID and member/admin role. The service SHALL generate a cryptographically random secret, show it only upon creation, store only its hash, and set expiry to 48 hours after creation. Creating an invitation for a current member SHALL fail with a conflict.

#### Scenario: Valid invitation
- **GIVEN** an authorized owner/admin and a nonmember target ID
- **WHEN** an invitation is created
- **THEN** a one-time secret and expiry are returned, while persistent records contain only its hash.

#### Scenario: Invalid invitation role or target
- **GIVEN** an owner-role invitation, nonpositive ID, member actor, or already-member target
- **WHEN** issuance is attempted
- **THEN** it is rejected without an invitation or success audit record.

### Requirement: One-time identity-bound acceptance
Acceptance SHALL require an authenticated user whose immutable GitHub ID matches the invitation target, matching secret, unexpired invitation, and unused/uncancelled status. Membership grant and invitation consumption SHALL commit atomically. A forwarded link SHALL NOT grant another account membership. Concurrent acceptance SHALL grant membership once.

#### Scenario: Intended account accepts
- **GIVEN** a valid invitation and its intended authenticated account
- **WHEN** acceptance succeeds
- **THEN** that user gains the recorded role and the invitation becomes consumed.

#### Scenario: Forwarded link
- **GIVEN** a valid secret opened by a different GitHub account
- **WHEN** it attempts acceptance
- **THEN** access is denied and the intended account can still accept the unused invitation.

#### Scenario: Expired or reused invitation
- **GIVEN** an expired, consumed, or wrong-secret invitation
- **WHEN** acceptance is attempted
- **THEN** no membership is granted.

#### Scenario: Concurrent acceptance
- **GIVEN** two simultaneous requests for the same valid invitation
- **WHEN** both attempt acceptance
- **THEN** exactly one membership grant and consumption commit.

### Requirement: Invitation cancellation and isolation
Owner/admin SHALL be able to cancel unused invitations in their workspace. Cancellation SHALL prevent later acceptance and be idempotent for an already-cancelled invitation. Members and administrators of other workspaces SHALL NOT view or cancel its invitations; invitation listings SHALL never redisplay secrets.

#### Scenario: Cancelled invitation
- **GIVEN** an unused invitation
- **WHEN** an authorized actor cancels it twice and the target later accepts
- **THEN** cancellation is idempotent and acceptance fails.

#### Scenario: Foreign workspace administration
- **GIVEN** an actor without membership in the invitation's workspace
- **WHEN** it lists or cancels that invitation
- **THEN** the service denies access without returning secret material.

### Requirement: Invitation audit metadata
Issuance, successful acceptance, and cancellation SHALL record corresponding audit metadata atomically with state changes. Audits SHALL identify the workspace, actor, target, invitation ID, action, and time without storing or logging the invitation secret.

#### Scenario: Invitation lifecycle auditing
- **GIVEN** a create/accept/cancel operation
- **WHEN** it commits or fails
- **THEN** committed lifecycle changes have matching safe audit events, while failed operations have no success events.
