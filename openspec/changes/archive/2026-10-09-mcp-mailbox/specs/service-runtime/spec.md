# Service Runtime Delta

## MODIFIED Requirements

### Requirement: Private features remain unavailable
Configured identity routes SHALL be available after identity initialization. Independently configured mailbox routes SHALL expose authenticated MCP at `/mcp`; without mailbox configuration that route SHALL return 404. Unimplemented management routes SHALL remain unavailable. If neither identity nor mailbox is configured, only public health endpoints SHALL be available.

#### Scenario: Premature MCP access
- **GIVEN** the foundation service is running
- **WHEN** a client sends an MCP initialization request to `/mcp`
- **THEN** it receives 404 and no MCP tool catalog.

#### Scenario: Configured identity routes
- **GIVEN** valid GitHub identity configuration
- **WHEN** a browser requests login or uses its valid session for workspace selection
- **THEN** the configured identity flow is available and only its own memberships are shown.

#### Scenario: Configured mailbox routes
- **GIVEN** valid mailbox cursor-signing configuration
- **WHEN** a native client connects with its own valid bearer credential
- **THEN** authenticated MCP is available independently of browser login.
