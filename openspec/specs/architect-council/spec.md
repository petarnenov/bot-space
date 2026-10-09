# architect-council Specification

## Purpose
Commit autonomous version-bound architect decisions with strict majority voting and bounded deliberation while retaining disagreement and provenance.

## Requirements

### Requirement: Fixed authorized voting council
Each decision SHALL snapshot its project-authorized architect members and input/proposal revision. Each member SHALL have one vote regardless of local model sessions. Votes SHALL authenticate the registered member and exact current proposal hash; stale, duplicate-identity and outsider votes SHALL not add support.

#### Scenario: Duplicate models or stale proposal
- **GIVEN** a fixed council and revised proposal
- **WHEN** one runner sends multiple opinions or an old-hash approval
- **THEN** it has at most one counted vote and stale approval cannot authorize the new proposal.

### Requirement: Strict majority and ties
Acceptance SHALL require floor(N/2)+1 approvals for the same proposal, where N is the fixed council size. Offline members and missing votes SHALL not shrink N or imply approval. Ties SHALL continue discussion; neither a coordinator nor HUMAN SHALL cast an overriding tie-break.

#### Scenario: Odd and even council sizes
- **GIVEN** councils of 2, 3, 4 and 6 members
- **WHEN** a proposal is considered
- **THEN** acceptance requires respectively 2, 2, 3 and 4 approvals, and equal support/opposition is not accepted.

### Requirement: Three rounds without minute limits
Each decision SHALL have at most three discussion rounds and no elapsed-time limit for deliberation. A round SHALL accept when a strict majority is reached or advance without acceptance after all members respond. After three unsuccessful rounds the decision SHALL be blocked_no_majority.

#### Scenario: Long discussion and round exhaustion
- **GIVEN** an unresolved decision
- **WHEN** significant time passes or a third round finishes without majority
- **THEN** time alone changes no votes or round state, while round exhaustion blocks execution.

### Requirement: Material reconsideration and autonomous authority
A blocked decision SHALL be reconsidered only for materially new input/evidence, proposal revision or explicit council-membership revision. Restarting identical rounds SHALL not evade the three-round cap. Architects SHALL decide plans, assignments, answers, reviews and authorized integration without human approval or override.

#### Scenario: Attempted endless restart
- **GIVEN** an exhausted decision and unchanged evidence
- **WHEN** a runner tries to reset its rounds
- **THEN** it remains blocked; materially changed input creates a traced reconsideration.

### Requirement: Durable decision commit and coordination
The server SHALL atomically validate and commit accepted proposals and vote evidence. A leased coordinator SHALL organize work without fabricating votes or committing unsupported decisions. Simultaneous coordinators SHALL not create conflicting authoritative acceptance for one decision revision.

#### Scenario: Concurrent coordinators
- **GIVEN** two coordinators attempting the same decision commit
- **WHEN** their requests race
- **THEN** one authoritative decision persists with validated majority evidence.

### Requirement: Executor question consultation
An authenticated assigned executor SHALL ask a question referencing its current work/spec revision. Architects SHALL discuss and vote on the exact answer through a separate decision channel. Accepted answers SHALL durably resume the correct executor session; unrelated, stale or unsupported answers SHALL not direct work.

#### Scenario: Executor waits for council
- **GIVEN** a running work package and question
- **WHEN** the council accepts an answer by strict majority
- **THEN** the executor receives that answer in its saved context and continues without HUMAN intervention.

### Requirement: Decision-scoped architect context
Each architect SHALL start a fresh model session for a new decision/question, seeded with the current task/spec/commit, the question, factual evidence and relevant accepted decisions. That session SHALL retain context through all discussion rounds for that decision. Parallel decisions SHALL have separate sessions. Final decisions, rationale and votes SHALL persist before working context is released.

#### Scenario: New question versus continuing discussion
- **GIVEN** an architect finishing one decision and receiving another question
- **WHEN** it starts the new discussion or continues the existing one
- **THEN** the new question gets a clean seeded session, while additional rounds of the same discussion resume its retained exact session.
