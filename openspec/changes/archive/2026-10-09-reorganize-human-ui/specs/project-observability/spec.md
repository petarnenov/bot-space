## MODIFIED Requirements

### Requirement: Human intake and autonomous status
The human UI SHALL create top-level intentions from the project dashboard and display autonomous progress, blocked reasons and outcomes in objective details. It SHALL not require per-task plan/retry/merge/deploy approval or offer a consensus override. Status SHALL distinguish deliberation, no-majority blocking, capacity waiting, execution, questions, verification and completion using recorded workflow state; unavailable progress SHALL be identified rather than fabricated.

#### Scenario: Blocked decision in human view
- **GIVEN** three unsuccessful council rounds
- **WHEN** the human views the story
- **THEN** blocked_no_majority is visible without an approval or tie-break request.

#### Scenario: No runtime evidence yet
- **GIVEN** a submitted objective with no recorded planning or execution evidence
- **WHEN** its dashboard row or detail page is viewed
- **THEN** submission and the known state are shown without claiming execution, completion, or a guessed completion percentage.

## ADDED Requirements

### Requirement: Unified objective detail
Each objective SHALL have one canonical detail page containing its title, description, current input revision, status, recorded progress, available result, and chronological history. Missing results or history SHALL have clear empty states. Existing creator lifecycle and revision controls SHALL remain available under current authorization. The page SHALL link back to its project's objectives and keep technical evidence secondary to the main summary.

#### Scenario: Inspect a completed objective
- **GIVEN** an authorized user and an objective with recorded outcome and history
- **WHEN** the user opens the objective from its project dashboard
- **THEN** description, completion state, available outcome, and ordered history are readable on that page without navigating to workspace administration.

#### Scenario: Technical details are secondary
- **GIVEN** an objective with authorized contracts, council decisions, assignments, or verification evidence
- **WHEN** its page is opened and technical details are expanded or followed
- **THEN** the main summary remains understandable and only evidence the current user may access is shown.

#### Scenario: Unrelated legacy task body
- **GIVEN** a project-authorized user who does not own a participant agent in a legacy delegated task
- **WHEN** the user views objective details or follows a delegated-task link
- **THEN** project access does not reveal that legacy task's protected instruction or result.

#### Scenario: Retained revisions and lifecycle
- **GIVEN** an existing objective with input revisions and lifecycle history
- **WHEN** the reorganized detail page is opened and an authorized creator action is submitted
- **THEN** existing history is preserved and the existing revision, lifecycle, authorization, and concurrency rules still apply.
