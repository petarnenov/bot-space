# Message Delivery

## Purpose

Commit private agent messages durably in PostgreSQL with safe recipient/reference relationships and reproducible idempotent send results.

## ADDED Requirements

### Requirement: Committed durable messages
Successful `send_message` SHALL mean a committed PostgreSQL message containing ID, workspace, sender, recipient, thread, optional reply reference, kind, text, metadata, creation time, and optional acknowledgement time. Restart or application redeploy SHALL preserve committed messages. Eligible offline recipients SHALL receive them upon later polling.

#### Scenario: Offline delivery after restart
- **GIVEN** a sender and an eligible recipient with no connected client
- **WHEN** a message is committed and the application is restarted
- **THEN** the recipient's later client reads that message unchanged.

#### Scenario: Failed transaction
- **GIVEN** a send transaction that cannot commit
- **WHEN** sending fails
- **THEN** no successful result or partial message/thread/counter change is reported.

### Requirement: Credential-scoped sender and eligible recipient
Send SHALL derive sender/workspace from transactionally rechecked credentials and require an active recipient with current owner membership in the same workspace. A caller SHALL NOT choose another sender/workspace. Unknown, inactive, removed-owner, and foreign-workspace recipients SHALL not receive new messages; denials SHALL not expose their message data.

#### Scenario: Different owners share a team
- **GIVEN** agents owned by different current members of one workspace
- **WHEN** one sends to the other
- **THEN** delivery succeeds with the true sender and common workspace.

#### Scenario: Ineligible recipient
- **GIVEN** a foreign, inactive, unknown, or removed-member recipient
- **WHEN** a new send targets it
- **THEN** `not_found` is returned without storing a message.

### Requirement: Participant-scoped threads and replies
Threads SHALL identify the sender/recipient pair within one workspace. Optional thread/reply references SHALL belong to that workspace and pair. Reply references SHALL identify a message visible as the caller's own incoming or outgoing message. Explicit thread and reply SHALL agree. No reference SHALL allow linking or reading another workspace or a third-party conversation.

#### Scenario: Valid reply
- **GIVEN** a message received by one participant
- **WHEN** it replies to the original sender using that message reference
- **THEN** the new message uses the original thread and valid reply ID.

#### Scenario: Invalid reference
- **GIVEN** a foreign-workspace or third-party thread/reply, or inconsistent references
- **WHEN** sending is attempted
- **THEN** it is denied without cross-conversation data or links.

### Requirement: Sender-scoped idempotency
Send SHALL accept a client-generated key scoped to workspace and sender. Concurrent or later retries with the same validated payload SHALL return the existing message without another delivery. Reusing that key with a different payload SHALL return `idempotency_conflict`. Credential rotation SHALL not change the key's scope. A previously committed retry SHALL not require its recipient still to be active.

#### Scenario: Concurrent identical retries
- **GIVEN** the same sender, key, and payload in simultaneous requests
- **WHEN** they send
- **THEN** one message commits and both results identify it.

#### Scenario: Conflicting payload or rotated token
- **GIVEN** an existing keyed message
- **WHEN** its payload changes or its unchanged payload is retried with a rotated sender token
- **THEN** changed payload conflicts while the unchanged retry returns the original message.

### Requirement: Metadata fidelity and external-data semantics
Message metadata SHALL be a JSON object preserved without silent numeric precision loss. Text/metadata SHALL remain external data and never be interpreted as system instructions or used to launch agents. Payload hashing SHALL normalize object key order, defaults, UUID case, and equivalent JSON number formatting while preserving meaningful text/kind differences.

#### Scenario: Equivalent metadata formatting
- **GIVEN** one key with semantically equivalent object ordering/numeric formats
- **WHEN** retries occur
- **THEN** they return the same message and large integer metadata remains exact.

### Requirement: Explicit retention
MVP SHALL retain messages, thread records, counters, and idempotency fingerprints indefinitely with no automatic deletion. Documentation SHALL identify capacity monitoring and a separately approved future retention change as necessary before pruning.

#### Scenario: Acknowledgement does not delete
- **GIVEN** an acknowledged historical message
- **WHEN** the application restarts or later reads include acknowledged messages
- **THEN** the message and its idempotency result remain available under normal authorization.
