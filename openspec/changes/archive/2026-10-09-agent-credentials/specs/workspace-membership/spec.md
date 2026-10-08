# Workspace Membership Delta

## ADDED Requirements

### Requirement: Removed member agent access termination
Removing a member SHALL atomically deactivate all its agents in that workspace and revoke their credentials while preserving agent identity records. Existing credentials SHALL NOT regain access if the user later rejoins. The transition SHALL be audited without secret material and SHALL NOT affect that user's agents in another workspace.

#### Scenario: Removal revokes all scoped agents
- **GIVEN** a member with agents and credentials in two workspaces
- **WHEN** its membership in one workspace is removed
- **THEN** credentials there fail immediately, historical agents remain, and credentials in the other workspace still work.

#### Scenario: Rejoining does not revive credentials
- **GIVEN** a removed member whose agents and credentials were disabled
- **WHEN** it rejoins through a new valid invitation
- **THEN** old tokens remain revoked and old agents remain inactive.
