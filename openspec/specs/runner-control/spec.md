# runner-control Specification

## Purpose
Provide authenticated typed machine control and durable event delivery over outbound connections without depending on an uninterrupted network stream.

## Requirements

### Requirement: Verified native gRPC control
Runner registration, presence, decisions, assignments, questions and results SHALL use an authenticated TLS-protected native gRPC control service. Public ingress SHALL be verified for unary calls, status trailers and bidirectional streaming before it is declared working. Existing mailbox MCP SHALL remain compatible.

#### Scenario: Public transport proof
- **GIVEN** a deployed control endpoint
- **WHEN** a native Go client exchanges unary and simultaneous stream traffic
- **THEN** authentication, trailers, delivery and reconnect work through the actual ingress.

### Requirement: Durable delivery and replay
Control events SHALL persist before acknowledgement. Each receiver SHALL track committed event IDs/cursors, deduplicate replay and recover after reconnect. Network transmission alone SHALL not count as task acceptance, execution or result delivery.

#### Scenario: Connection lost around acknowledgement
- **GIVEN** an event sent near a disconnect
- **WHEN** the runner reconnects
- **THEN** committed events replay safely and unacknowledged delivery does not become a false completion.

### Requirement: Bounded transport independent of deliberation
RPC input/output, buffering, retries, backpressure and network liveness SHALL be bounded. Transport timeout SHALL not approve votes, close a deliberation round or shrink a council. Model sessions waiting on council answers SHALL preserve their exact correlation and continue network processing.

#### Scenario: Slow architect versus broken transport
- **GIVEN** a long deliberation and a reconnecting runner
- **WHEN** a technical call or connection times out
- **THEN** the durable decision remains pending without a discussion-minute deadline or invented vote.
