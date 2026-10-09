# MCP Mailbox Tools Delta

## MODIFIED Requirements

### Requirement: Executable two-client request reply
The repository SHALL retain a runnable mailbox example with separate requester/
responder processes, separate credentials, and bounded inbox polling through the
official SDK. Documentation SHALL also describe automatic task execution and
sender continuation through one managed runner per physical machine, while
explaining that unmanaged inactive Codex/Claude/Copilot sessions are not
awakened by ordinary messages.

#### Scenario: Request reply demo
- **GIVEN** two eligible agents and their independent configured client
  processes
- **WHEN** requester sends and both poll
- **THEN** responder reads, acknowledges and replies, and requester reads the
  reply over HTTP MCP.

#### Scenario: Managed delegated execution
- **GIVEN** two physical machines with authenticated managed runners
- **WHEN** one agent delegates work to the other
- **THEN** the recipient executes automatically and the originating session
  continues with its correlated result.
