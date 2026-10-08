# Service Runtime Delta

## MODIFIED Requirements

### Requirement: Private features remain unavailable
Configured GitHub login/callback/logout and authenticated workspace-selection routes SHALL be available after identity initialization. `/mcp` and unimplemented management routes SHALL return HTTP 404 without tools, messages, or account actions. Without complete identity configuration, only public health endpoints SHALL be available.

#### Scenario: Premature MCP access
- **GIVEN** the foundation service is running
- **WHEN** a client sends an MCP initialization request to `/mcp`
- **THEN** it receives 404 and no MCP tool catalog.

#### Scenario: Configured identity routes
- **GIVEN** valid GitHub identity configuration
- **WHEN** a browser requests login or uses its valid session for workspace selection
- **THEN** the configured identity flow is available and only its own memberships are shown.
