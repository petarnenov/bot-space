## MODIFIED Requirements

### Requirement: Human-originated root intentions
Only an authenticated current human with project access, or a trusted database operator acting for that existing human by immutable GitHub ID, SHALL create a root intention. Runner credentials SHALL not authenticate intake. Root records SHALL retain immutable creator, project, original description, priority and revision provenance. Operator intake SHALL enforce current repository authority and the same validation and audit history as browser intake.

#### Scenario: Human intake versus runner impersonation
- **GIVEN** an authorized human and a registered runner
- **WHEN** each submits a root objective
- **THEN** human intake persists its provenance and the runner cannot impersonate a human creator.

#### Scenario: Trusted operator intake
- **GIVEN** database access, an existing human GitHub identity and that human's current project authority
- **WHEN** an operator submits an objective through the supported CLI command
- **THEN** the shared queue records the human as creator with the same revision and audit provenance as browser intake.

#### Scenario: Unauthorized operator intake
- **GIVEN** an unknown GitHub identity or a human without current project authority
- **WHEN** an operator attempts CLI intake
- **THEN** the command fails without creating an objective, revision or audit event.
