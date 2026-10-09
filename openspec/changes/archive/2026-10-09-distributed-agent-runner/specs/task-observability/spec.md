# Task Observability

## Purpose

Expose useful delegated-work status and authorized results while preserving workspace isolation and private agent content.

## ADDED Requirements

### Requirement: Participant-owned task views
Authenticated humans SHALL list and inspect tasks involving their own agents in their workspace, including instructions, result and bounded failure details. Owner/admin role SHALL not grant access to other agents' task bodies. Task identifiers SHALL not bypass participant ownership or current membership.

#### Scenario: Human reviews delegated work
- **GIVEN** a task between two users' agents
- **WHEN** either participant's owner or an unrelated administrator opens it
- **THEN** participant owners see the task while the unrelated administrator receives no content.

### Requirement: Execution status and presence
Views SHALL distinguish queued, running, waiting_dependency, completed, failed, cancelled, expired, interrupted and requires_approval states. Heartbeats SHALL expose last-seen status without implying task success; more than 90 seconds without heartbeat SHALL indicate offline. Offline tasks SHALL remain durable until deadline or explicit action.

#### Scenario: Recipient machine offline
- **GIVEN** a recipient with no recent heartbeat
- **WHEN** a task is submitted or viewed
- **THEN** it remains queued with an offline indication and never appears falsely completed.

### Requirement: Protected task controls and safe audit
Browser cancellation and review/retry actions SHALL require current participant authorization, CSRF and same-origin checks. Effective task transitions SHALL have atomic audit metadata without prompts/results/tokens. Views SHALL escape external content and retain no-store/no-referrer. Operational logs SHALL contain identifiers/status only.

#### Scenario: Forged control or unsafe content
- **GIVEN** an unauthorized mutation or script-like result
- **WHEN** task controls or views process it
- **THEN** mutation is denied or content escaped, and private payloads do not enter logs or audit.
