# Project Observability

## Purpose

Expose the human backlog, architect decisions and executor evidence under explicit project access while preserving private unrelated data.

## ADDED Requirements

### Requirement: Project-authorized workflow views
Authorized project users and architects SHALL see the backlog, OpenSpec revisions, council membership/proposals/votes, assignments, questions and evidence permitted for that project. Executors SHALL receive their assigned context. Unrelated projects/workspaces and ordinary private inboxes SHALL remain isolated.

#### Scenario: Council reviews another executor
- **GIVEN** a project-authorized architect and an unrelated caller
- **WHEN** they inspect an executor's project work
- **THEN** the architect sees the required context and the unrelated caller receives no content.

### Requirement: Human intake and autonomous status
The human UI SHALL create top-level intentions and display autonomous progress, blocked reasons and outcomes. It SHALL not require per-task plan/retry/merge/deploy approval or offer a consensus override. Status SHALL distinguish deliberation, no-majority blocking, capacity waiting, execution, questions, verification and completion.

#### Scenario: Blocked decision in human view
- **GIVEN** three unsuccessful council rounds
- **WHEN** the human views the story
- **THEN** blocked_no_majority is visible without an approval or tie-break request.

### Requirement: Safe audit and rendering
Identity, intake, votes, assignments and authoritative transitions SHALL have safe audit metadata. Browser mutations SHALL retain CSRF/same-origin protection. Views SHALL escape external content and use no-store/no-referrer; credentials and task bodies SHALL not enter operational logs.

#### Scenario: Unsafe content or forged browser mutation
- **GIVEN** script-like external text or invalid CSRF
- **WHEN** it is rendered or submitted
- **THEN** text is escaped or mutation denied without exposing secrets in logs.

### Requirement: Product name
The human interface SHALL display the product name “The Firm” in its shared header and page-title branding. Repository/module identifiers and existing workspace slugs SHALL retain compatibility.

#### Scenario: Branded pages
- **GIVEN** a human opening the application
- **WHEN** shared pages or agent credential results render
- **THEN** the displayed application brand is “The Firm”.
