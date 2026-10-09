# MCP Mailbox Tools Delta

## MODIFIED Requirements

### Requirement: Stable application failures
Application failures SHALL return bounded tool error data with stable codes: invalid_argument, not_found, forbidden, idempotency_conflict, rate_limited, and temporarily_unavailable. Additive task tools SHALL also use lease_conflict and dependency_conflict. Private inputs and raw SQL errors SHALL NOT be echoed. SDK protocol errors SHALL remain SDK-defined.

#### Scenario: Application or protocol error
- **GIVEN** invalid application data or a malformed protocol request
- **WHEN** MCP processes it
- **THEN** application failures use the documented code envelope, while protocol failures retain SDK semantics, without private input disclosure.

### Requirement: Executable two-client request reply
The repository SHALL retain a runnable mailbox example with separate requester/responder processes, separate credentials, and bounded inbox polling through the official SDK. Documentation SHALL also describe automatic task execution and sender continuation through one managed runner per physical machine, while explaining that unmanaged inactive Codex/Claude/Copilot sessions are not awakened by ordinary messages.

#### Scenario: Request reply demo
- **GIVEN** two eligible agents and their independent configured client processes
- **WHEN** requester sends and both poll
- **THEN** responder reads, acknowledges and replies, and requester reads the reply over HTTP MCP.

#### Scenario: Managed delegated execution
- **GIVEN** two physical machines with authenticated managed runners
- **WHEN** one agent delegates work to the other
- **THEN** the recipient executes automatically and the originating session continues with its correlated result.
