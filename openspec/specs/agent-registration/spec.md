# agent-registration Specification

## Purpose

Associate each provider-neutral agent with an authenticated human owner and isolated workspace, with explicit activation policy.

## Requirements

### Requirement: Scoped agent registration
Active workspace members SHALL register agents owned by themselves in that workspace. Agents SHALL have independent UUID identities and display names of 1–128 UTF-8 bytes without control characters. The server SHALL derive ownership from the authenticated human, reject foreign-workspace registration, and treat activation as permission to participate rather than online presence.

#### Scenario: Different members register agents
- **GIVEN** two members of the same workspace
- **WHEN** each registers an agent
- **THEN** distinct agent IDs persist with their respective owners and common workspace.

#### Scenario: Invalid or foreign registration
- **GIVEN** an invalid name or no caller membership in the chosen workspace
- **WHEN** registration is attempted
- **THEN** no agent is created.

### Requirement: Own-agent management
Members SHALL list and manage only their own agents through personal management operations. Another member's agent SHALL NOT be manageable by substituting its ID. Display names SHALL NOT identify or constrain a model/provider; an eligible offline agent SHALL remain registered for later mailbox delivery.

#### Scenario: Own-agent listing
- **GIVEN** several members' agents in a workspace
- **WHEN** one member lists personal agents
- **THEN** only its own agents are returned.

#### Scenario: Substituted foreign agent
- **GIVEN** a member with another member's agent ID
- **WHEN** it attempts unauthorized management
- **THEN** the server denies the action without changing the agent.

### Requirement: Agent deactivation
A member SHALL deactivate its own agents. Owner/admin SHALL deactivate any agent in their workspace. Deactivation SHALL atomically mark the agent inactive and revoke all its credentials, be idempotent, and prevent future credential issuance or authentication. It SHALL NOT delete historical identity records or grant access to mailbox content.

#### Scenario: Administrator deactivates a team agent
- **GIVEN** an owner/admin and another member's active agent
- **WHEN** deactivation commits
- **THEN** every credential is unusable on subsequent requests and the historical agent record remains.

#### Scenario: Foreign administrator
- **GIVEN** an administrator from another workspace
- **WHEN** it attempts deactivation
- **THEN** the action is denied.

### Requirement: Agent lifecycle audit
Registration and effective deactivation SHALL persist safe audit metadata atomically with their changes. Repeating deactivation SHALL NOT create another effective transition event. Events SHALL identify actor, workspace, agent, action, and time without tokens or message content.

#### Scenario: Audited lifecycle
- **GIVEN** successful registration/deactivation or a failed operation
- **WHEN** domain and audit records are inspected
- **THEN** committed transitions have matching safe events and rejected operations have no success events.
