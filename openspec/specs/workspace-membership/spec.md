# workspace-membership Specification

## Purpose

Restrict team access to explicit memberships and preserve authorized workspace administration under concurrent changes.

## Requirements

### Requirement: Isolated multiple memberships
A user SHALL be able to belong to multiple isolated workspaces, with one role per membership: owner, admin, or member. Workspace listing and member listing SHALL return only workspaces accessible to the caller. No public endpoint SHALL create a workspace or grant membership without an invitation.

#### Scenario: Multiple teams
- **GIVEN** a user with membership in two workspaces and no membership in a third
- **WHEN** the user lists workspaces or requests third-workspace members
- **THEN** only the two memberships are listed and third-workspace access is denied.

### Requirement: Administrative bootstrap
`mailbox bootstrap-owner --github-user-id <positive-id> --workspace <slug>` SHALL create or locate the specified immutable identity, create its workspace, assign the first owner, and audit the operation transactionally. Repeating a matching bootstrap SHALL be idempotent. A conflicting existing workspace SHALL be refused rather than silently granting ownership.

#### Scenario: First owner and repeat
- **GIVEN** no workspace with the requested slug
- **WHEN** bootstrap runs twice with the same GitHub ID and slug
- **THEN** one workspace and owner membership exist, with no duplicate grant.

#### Scenario: Conflicting bootstrap
- **GIVEN** the slug belongs to a workspace bootstrapped for another identity
- **WHEN** bootstrap targets that slug for a different account
- **THEN** it fails nonzero without granting membership.

### Requirement: Role administration
Only owner/admin SHALL invite and remove members. Admin SHALL NOT remove or change an owner's role or grant ownership to any account. Only owners SHALL grant owner role. Members SHALL NOT alter roles or other memberships. Every mutation SHALL recheck actor membership and role within its transaction.

#### Scenario: Admin attempts owner escalation
- **GIVEN** an admin session
- **WHEN** it attempts self-promotion to owner or removal/demotion of an owner
- **THEN** authorization rejects the mutation and roles remain unchanged.

#### Scenario: Member attempts administration
- **GIVEN** a member session
- **WHEN** it attempts invitations, role changes, or member removal
- **THEN** the action is denied on the server.

### Requirement: Last owner invariant
Removal or demotion of the last workspace owner SHALL be refused, including when several owner-changing transactions run concurrently. The successful final state SHALL always retain at least one owner.

#### Scenario: Last owner removal
- **GIVEN** exactly one workspace owner
- **WHEN** an authorized actor attempts its removal or demotion
- **THEN** the operation is rejected without changing ownership.

#### Scenario: Concurrent owner changes
- **GIVEN** two owners
- **WHEN** concurrent operations would remove or demote both
- **THEN** at most one succeeds and at least one owner remains.

### Requirement: Transactional role auditing
Bootstrap, successful role changes, and member removals SHALL persist audit events in the same transaction as the domain change. Events SHALL contain actor/target IDs, workspace, action, and time, without tokens, invitation secrets, or message content.

#### Scenario: Audit and state agree
- **GIVEN** a successful membership mutation or a transaction that rolls back
- **WHEN** state and audit records are inspected
- **THEN** committed mutations have corresponding events and rolled-back mutations have neither changes nor success events.
