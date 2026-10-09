# request-limits Specification

## Purpose

Bound request admission and limiter memory for the private service while preserving health diagnostics and tenant authorization.

## Requirements

### Requirement: Bounded per-peer admission
Login initiation SHALL allow 20 requests/minute per network peer with burst 5; MCP admission SHALL allow 120/minute per peer with burst 60. Admission SHALL occur before body buffering or database authentication. Health endpoints SHALL remain available independently of these buckets. Limiter state SHALL cap at 10000 keys and reclaim entries idle for ten minutes.

#### Scenario: Excess login or MCP traffic
- **GIVEN** a peer exceeding its bucket
- **WHEN** another request arrives
- **THEN** HTTP 429 and bounded Retry-After are returned before protected work, while health requests remain available.

### Requirement: Agent request rate
Authenticated MCP requests SHALL additionally share one 60/minute bucket with burst 20 per agent identity in the replica, independent of credential rotation or multiple clients. Rate rejection SHALL return HTTP 429 with `rate_limited` error data and SHALL NOT execute a tool.

#### Scenario: Two clients share a limit
- **GIVEN** two clients using one agent identity
- **WHEN** their combined requests exceed its burst/refill
- **THEN** excess calls are rejected without message writes.

### Requirement: Replica and proxy semantics
Documentation SHALL state that limits are per replica and key peer admission by the actual network peer unless a separately verified proxy policy is configured. Caller-supplied forwarding headers SHALL NOT bypass limits. MVP Railway configuration SHALL retain one replica; distributed rate guarantees SHALL not be claimed.

#### Scenario: Spoofed forwarding header
- **GIVEN** one peer varying X-Forwarded-For
- **WHEN** it exceeds admission limits
- **THEN** the same bucket still applies and documented single-replica semantics match configuration.
