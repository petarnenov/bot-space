# Specification Coverage

Every current requirement and its named scenarios are indexed below. Evidence
files contain the executed assertions or inspected deployment/documentation
artifact. The [verification report](verification.md) records current commands,
outcomes and scope; historical archive reports explain individual tests.
An index entry alone is not proof of execution.

## agent-credentials

Evidence: [tests/integration/agents_test.go](../tests/integration/agents_test.go), [tests/integration/agents_http_test.go](../tests/integration/agents_http_test.go), [internal/security/security_test.go](../internal/security/security_test.go), [docs/agents.md](../docs/agents.md).

| Requirement | Scenarios checked |
| --- | --- |
| One-time high-entropy credentials | Separate credentials; Credential listing and cookie-only request |
| Credential ownership | Administrator attempts another owner's issuance |
| Uncached agent authentication | Credential-derived identity; Missing invalid or revoked credentials |
| Atomic revocation and rotation | Rotation replaces one credential; Repeated revocation |
| Transaction-bound authorization recheck | In-flight transaction and revocation |
| Credential audit and interoperability limits | Safe credential lifecycle record |

## agent-registration

Evidence: [tests/integration/agents_test.go](../tests/integration/agents_test.go), [tests/integration/agents_http_test.go](../tests/integration/agents_http_test.go), [tests/integration/management_test.go](../tests/integration/management_test.go).

| Requirement | Scenarios checked |
| --- | --- |
| Scoped agent registration | Different members register agents; Invalid or foreign registration |
| Own-agent management | Own-agent listing; Substituted foreign agent |
| Agent deactivation | Administrator deactivates a team agent; Foreign administrator |
| Agent lifecycle audit | Audited lifecycle |

## database-lifecycle

Evidence: [tests/integration/database_test.go](../tests/integration/database_test.go), [tests/integration/process_restart_test.go](../tests/integration/process_restart_test.go), [tests/integration/backup_test.go](../tests/integration/backup_test.go), [scripts/foundation-smoke.py](../scripts/foundation-smoke.py).

| Requirement | Scenarios checked |
| --- | --- |
| Durable PostgreSQL state | Application container replacement |
| Explicit migration command | Repeated migration; Service without migration |
| Serialized and atomic migrations | Two simultaneous migrators; Migration failure; Lock contention deadline |
| Migration integrity | Applied migration edited; Older binary against newer schema |
| Bounded database connections | Pool exhaustion |

## deployment-runtime

Evidence: [Dockerfile](../Dockerfile), [compose.yaml](../compose.yaml), [.github/workflows/ci.yml](../.github/workflows/ci.yml), [docs/railway.md](../docs/railway.md), [docs/verification.md](../docs/verification.md).

| Requirement | Scenarios checked |
| --- | --- |
| Reproducible non-root container | Build and run |
| Local persistent development environment | Ordinary stop and restart |
| Railway deployment preparation | Prepared configuration review |
| Foundation CI checks | Verification failure blocks CI |
| Public repository documentation | Fresh-machine foundation setup; Secret file exclusion |

## human-authentication

Evidence: [tests/integration/identity_test.go](../tests/integration/identity_test.go), [internal/identity/web_test.go](../internal/identity/web_test.go), [internal/config/identity_test.go](../internal/config/identity_test.go), [internal/security/security_test.go](../internal/security/security_test.go).

| Requirement | Scenarios checked |
| --- | --- |
| GitHub identity | Username changes; New login has no team access |
| OAuth state and PKCE | Successful protected flow; Invalid or replayed state |
| Bounded OAuth provider calls | Provider error or invalid identity |
| Server-side browser sessions | Session fixation prevention; Expired session |
| Logout and browser mutation protection | Logout revokes a cookie; Forged mutation |
| Configuration and safe redirects | Unsafe return target; Disabled or incomplete configuration |

## inbox-processing

Evidence: [tests/integration/mailbox_test.go](../tests/integration/mailbox_test.go), [tests/integration/mailbox_concurrency_test.go](../tests/integration/mailbox_concurrency_test.go), [internal/mailbox/cursors_test.go](../internal/mailbox/cursors_test.go), [tests/integration/mcp_more_test.go](../tests/integration/mcp_more_test.go).

| Requirement | Scenarios checked |
| --- | --- |
| Own inbox and nondestructive reads | Repeated own-inbox read; Foreign inbox attempt |
| Idempotent own acknowledgement | Repeated acknowledgement; Nonrecipient acknowledgement |
| Commit-ordered inbox sequences | Concurrent commits |
| Signed snapshot pagination | Insert between pages; Cursor misuse |
| Live acknowledgement filtering | Another client acknowledges during paging |
| Shared identity and repeated processing | Two clients share one identity |

## mcp-mailbox-tools

Evidence: [tests/integration/mcp_test.go](../tests/integration/mcp_test.go), [tests/integration/mcp_more_test.go](../tests/integration/mcp_more_test.go), [tests/integration/mcp_concurrency_test.go](../tests/integration/mcp_concurrency_test.go), [tests/integration/example_test.go](../tests/integration/example_test.go), [internal/mailbox/validation_test.go](../internal/mailbox/validation_test.go).

| Requirement | Scenarios checked |
| --- | --- |
| Official authenticated remote transport | Two independent clients; Unauthorized protocol request |
| MCP Origin and body protection | Untrusted browser origin; Native client and oversized body |
| Credential-derived identity and recipient directory | Isolated directory and identity |
| Explicit mailbox tool schemas | Spoofed tool identity |
| Tool limits | Invalid payload or page limit; Large inbox page |
| Stable application failures | Application or protocol error |
| Executable two-client request reply | Request reply demo |

## message-delivery

Evidence: [tests/integration/mailbox_test.go](../tests/integration/mailbox_test.go), [tests/integration/mailbox_concurrency_test.go](../tests/integration/mailbox_concurrency_test.go), [tests/integration/process_restart_test.go](../tests/integration/process_restart_test.go), [internal/mailbox/validation_test.go](../internal/mailbox/validation_test.go), [tests/integration/mcp_more_test.go](../tests/integration/mcp_more_test.go).

| Requirement | Scenarios checked |
| --- | --- |
| Committed durable messages | Offline delivery after restart; Failed transaction |
| Credential-scoped sender and eligible recipient | Different owners share a team; Ineligible recipient |
| Participant-scoped threads and replies | Valid reply; Invalid reference |
| Sender-scoped idempotency | Concurrent identical retries; Conflicting payload or rotated token |
| Metadata fidelity and external-data semantics | Equivalent metadata formatting |
| Explicit retention | Acknowledgement does not delete |

## release-readiness

Evidence: [tests/integration/management_test.go](../tests/integration/management_test.go), [tests/integration/process_restart_test.go](../tests/integration/process_restart_test.go), [tests/integration/client_cli_test.go](../tests/integration/client_cli_test.go), [tests/integration/backup_test.go](../tests/integration/backup_test.go), [.github/workflows/ci.yml](../.github/workflows/ci.yml), [docs/verification.md](../docs/verification.md).

| Requirement | Scenarios checked |
| --- | --- |
| Complete browser to MCP acceptance | Complete invited-user exchange |
| Verified client and activation documentation | Reproducible client configuration |
| Backup restore and Railway preparation | Isolated restore drill |
| Public source and CI evidence | Public release candidate |
| Full fresh checkout reproduction and audit | Final readiness review |

## request-limits

Evidence: [internal/ratelimit/limit_test.go](../internal/ratelimit/limit_test.go), [tests/integration/rate_limit_test.go](../tests/integration/rate_limit_test.go), [tests/integration/proxy_host_test.go](../tests/integration/proxy_host_test.go), [docs/mcp.md](../docs/mcp.md), [railway.toml](../railway.toml).

| Requirement | Scenarios checked |
| --- | --- |
| Bounded per-peer admission | Excess login or MCP traffic |
| Agent request rate | Two clients share a limit |
| Replica and proxy semantics | Spoofed forwarding header |

## service-runtime

Evidence: [internal/config/config_test.go](../internal/config/config_test.go), [cmd/mailbox/main_test.go](../cmd/mailbox/main_test.go), [internal/httpserver/server_test.go](../internal/httpserver/server_test.go), [internal/httpserver/shutdown_test.go](../internal/httpserver/shutdown_test.go), [tests/integration/database_test.go](../tests/integration/database_test.go), [tests/integration/mcp_test.go](../tests/integration/mcp_test.go).

| Requirement | Scenarios checked |
| --- | --- |
| Environment configuration | Railway supplied port; Invalid configuration |
| Liveness and readiness | Database unavailable after startup; Required schema missing |
| Bounded HTTP handling | Oversized body; Incomplete headers |
| Private features remain unavailable | Premature MCP access; Configured identity routes; Configured mailbox routes |
| Safe operational logging | Sensitive failure input |
| Graceful shutdown | Shutdown during a request; Shutdown deadline exceeded |

## web-management

Evidence: [tests/integration/management_test.go](../tests/integration/management_test.go), [tests/integration/agents_http_test.go](../tests/integration/agents_http_test.go), [tests/integration/invitation_http_test.go](../tests/integration/invitation_http_test.go), [internal/web/management.go](../internal/web/management.go).

| Requirement | Scenarios checked |
| --- | --- |
| Authenticated navigation and member administration | Role-aware administration |
| Invitation management and identity-bound acceptance | Invitation link lifecycle |
| Owned agents and one-time credential pages | Owned credential workflow |
| Private escaped browser rendering | Administrative privacy and escaping |

## workspace-invitations

Evidence: [tests/integration/workspaces_test.go](../tests/integration/workspaces_test.go), [tests/integration/invitation_http_test.go](../tests/integration/invitation_http_test.go), [tests/integration/management_test.go](../tests/integration/management_test.go).

| Requirement | Scenarios checked |
| --- | --- |
| Targeted invitation issuance | Valid invitation; Invalid invitation role or target |
| One-time identity-bound acceptance | Intended account accepts; Forwarded link; Expired or reused invitation; Concurrent acceptance |
| Invitation cancellation and isolation | Cancelled invitation; Foreign workspace administration |
| Invitation audit metadata | Invitation lifecycle auditing |

## workspace-membership

Evidence: [tests/integration/workspaces_test.go](../tests/integration/workspaces_test.go), [tests/integration/agents_test.go](../tests/integration/agents_test.go), [tests/integration/management_test.go](../tests/integration/management_test.go), [cmd/mailbox/main.go](../cmd/mailbox/main.go).

| Requirement | Scenarios checked |
| --- | --- |
| Isolated multiple memberships | Multiple teams |
| Administrative bootstrap | First owner and repeat; Conflicting bootstrap |
| Role administration | Admin attempts owner escalation; Member attempts administration |
| Last owner invariant | Last owner removal; Concurrent owner changes |
| Transactional role auditing | Audit and state agree |
| Removed member agent access termination | Removal revokes all scoped agents; Rejoining does not revive credentials |

Inventory: 14 capabilities, 73 requirements, 114 scenarios.
