# Agent Credential Verification

Verified locally on 2026-10-09 (Europe/Sofia), using Go 1.27.2 and real PostgreSQL 18.6. All checks use synthetic users/messages/credentials; no public deployment occurred.

## Executed Checks

- Full `go test -race -count=1 ./...` with real PostgreSQL: pass, including prior foundation/identity tests and actual browser mutation routes.
- Formatting, `go mod verify`, vet, build, and strict OpenSpec validation: pass.
- govulncheck v1.8.0: no vulnerabilities found.
- Docker build: pass. New image applies the additive third migration and serves configured identity routes. Unauthenticated agent mutation returns 401, and `/mcp` remains 404.
- Foundation lifecycle smoke on the new image: pass, including application replacement, named-volume persistence, and database outage/recovery.

## Requirement and Scenario Evidence

| Requirement | Evidence |
| --- | --- |
| Scoped agent registration | `TestAgentOwnershipCredentialHashesAndDeactivation`: distinct members create distinct owned UUID agents; invalid UTF-8/control/empty/overlong names and foreign-workspace registration are denied. `TestAgentBrowserMutationsUseSessionOwnership` proves actual POST registration ignores a spoofed owner field and requires CSRF. |
| Own-agent management | Personal listing includes only the caller's agent; substituted foreign agent management is denied. Native/HTTP owner checks requery workspace membership rather than trusting a submitted actor. |
| Agent deactivation | Same-team administrator deactivates another member's agent; foreign administrator/member substitution is denied. All credentials fail, inactive issuance/rotation fail, repeat deactivation is idempotent, and historical rows remain. Browser deactivation also exercises the real route. |
| Agent lifecycle audit | Registration/issuance/revocation/deactivation counts match committed transitions. `TestAgentAuditFailureRollsBack` forces audit insertion failure and proves registration, rotation, deactivation and removal cannot partially commit. |
| One-time high-entropy credentials | Tokens decode to 32 random bytes, are independent per agent, and stored hashes equal SHA-256 while plaintext is absent. Credential metadata JSON contains no token/hash. Browser result page returns only newly issued/rotated tokens with no-store/no-referrer. A valid browser-session cookie alone cannot authenticate the agent HTTP probe. |
| Credential ownership | Same-workspace owner/admin cannot issue/list/rotate another member's credential. Browser issuance likewise denies an administrator despite its valid session and CSRF. |
| Uncached agent authentication | `TestAgentHTTPAuthenticationBoundary` exercises real HTTP: valid principal comes from the credential despite spoofed sender/workspace parameters; missing/basic/malformed/unknown/duplicate/cookie-only requests never run the protected handler; revoked token gives 401; database failure gives 503 without access. |
| Atomic revocation and rotation | `TestCredentialRotationRevocationAndAudit` proves old token denial, replacement validity, unaffected other credential, idempotent revoke, and denied revoked/inactive rotation. Audit failure rolls back rotation, keeping the original credential and no extra credential row. Real browser issuance/rotation/revoke verifies the same lifecycle over protected POST routes. |
| Transaction-bound authorization recheck | `TestCredentialTransactionLockAndStaleContext/revoke`, `/deactivate`, and `/remove` hold validated transaction locks and observe actual PostgreSQL `wait_event_type='Lock'`; after commit and access termination, new authentication and stale-principal rechecks fail. |
| Credential audit and interoperability limits | Lifecycle tests check effective audit counts and absence of raw token/hash material. `docs/agents.md` explicitly distinguishes bearer credentials from full MCP OAuth and test-only probe checks from later SDK/client interoperability. |
| Removed member agent access termination | `TestRemovalPermanentRevocationAndWorkspaceIsolation` removes a member with multiple agents/credentials in one team: every affected token fails, another team's token works, agent records remain, safe actor-attributed audit events exist, and rejoining via a fresh invitation never revives old access. Trigger-backed changes roll back if audit insertion fails. |

## Scope and External Limits

The production application now registers protected browser POST agent/credential routes under configured GitHub identity. Listing/management forms are completed in change 5. Agent HTTP middleware is verified on a test-only probe; production MCP mounting/tools and the executable two-client exchange are change 4. No live GitHub OAuth, hosted CI, public GitHub publication, paid provisioning, or Railway deployment is claimed.
