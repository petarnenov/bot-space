# Inbox Processing

## Purpose

Let agents read and acknowledge only their own inbox with bounded, stable delivery pagination and honest repeated-processing semantics.

## ADDED Requirements

### Requirement: Own inbox and nondestructive reads
`read_messages` SHALL return only messages addressed to the authenticated agent in its workspace, with acknowledgement/thread/kind filters. Default acknowledgement filter SHALL be unacknowledged. Reading SHALL neither acknowledge nor delete messages. Another agent's inbox SHALL NOT be selectable by parameters or reused cursors, including by a human administrator's agent.

#### Scenario: Repeated own-inbox read
- **GIVEN** an unacknowledged incoming message
- **WHEN** its recipient reads twice without acknowledgement
- **THEN** both reads can return it and processing state remains unacknowledged.

#### Scenario: Foreign inbox attempt
- **GIVEN** another agent's inbox ID or cursor
- **WHEN** the caller attempts to read it
- **THEN** the request is denied without its messages.

### Requirement: Idempotent own acknowledgement
`acknowledge_message` SHALL update only an incoming message in the authenticated agent's workspace/inbox. Repeating acknowledgement SHALL return the original acknowledgement timestamp. A sender or another agent SHALL NOT acknowledge someone else's incoming message. Acknowledgement SHALL not delete data.

#### Scenario: Repeated acknowledgement
- **GIVEN** an incoming message
- **WHEN** the recipient acknowledges twice
- **THEN** both results have the same non-null acknowledgement timestamp and the message remains stored.

#### Scenario: Nonrecipient acknowledgement
- **GIVEN** a known message ID addressed to another agent
- **WHEN** the caller acknowledges it
- **THEN** `not_found` is returned without changing the message.

### Requirement: Commit-ordered inbox sequences
Each recipient inbox SHALL order messages by a monotonically increasing sequence whose increment commits atomically with the message. A higher sequence SHALL NOT commit before a lower one. Message UUID/creation time SHALL not be used as a substitute for this concurrent delivery ordering.

#### Scenario: Concurrent commits
- **GIVEN** simultaneous sends to one recipient with a delayed earlier transaction
- **WHEN** they commit
- **THEN** delivery sequences follow commit order and pagination cannot skip a later-committing lower sequence.

### Requirement: Signed snapshot pagination
A fresh read SHALL capture the highest committed inbox sequence as an upper bound. Continuation cursors SHALL be opaque, signed, bounded, and tied to agent/workspace plus filters, advancing strictly past returned sequences. Inserts beyond that upper bound SHALL appear on the next fresh poll. Invalid, tampered, foreign, or differently filtered cursors SHALL return `invalid_argument`.

#### Scenario: Insert between pages
- **GIVEN** a paginated inbox snapshot
- **WHEN** a new message commits before a continuation read
- **THEN** existing snapshot pages remain ordered without duplicates/skips and the new message is returned on a fresh poll.

#### Scenario: Cursor misuse
- **GIVEN** a tampered/foreign cursor or changed filters
- **WHEN** continuation is requested
- **THEN** no messages are returned and `invalid_argument` is reported.

### Requirement: Live acknowledgement filtering
Pagination SHALL snapshot delivery eligibility by sequence, while acknowledgement filters SHALL be evaluated when each page is read. Acknowledgements by another client can remove entries from an unacknowledged scan. This SHALL NOT be represented as a historical snapshot of processing state.

#### Scenario: Another client acknowledges during paging
- **GIVEN** an unacknowledged delivery snapshot
- **WHEN** another client acknowledges an unseen entry before its page
- **THEN** that entry can disappear from the unacknowledged page and remains available with an acknowledged/all filter.

### Requirement: Shared identity and repeated processing
Multiple clients with one agent identity SHALL be allowed to read the same messages and independently acknowledge them. The service SHALL provide no claim/lease, single-consumer guarantee, or exactly-once recipient execution. Unacknowledged data SHALL remain available on later fresh polls.

#### Scenario: Two clients share one identity
- **GIVEN** two independently connected clients using one agent's credentials
- **WHEN** both read before acknowledgement
- **THEN** both can receive the same message and must tolerate duplicate processing.
