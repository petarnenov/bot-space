# MCP Mailbox Verification

Verified locally on 2026-10-09 with Go 1.27.2, official MCP Go SDK v1.8.0, and real PostgreSQL 18.6. No substitute direct-function test is used for the required HTTP/client proof.

## Executed Checks

- Formatting includes `cmd internal migrations tests examples`; module verification, vet and build pass.
- Full `go test -race -count=1 ./...` with real PostgreSQL passes, including all earlier changes, HTTP MCP, executable clients, and actual process restart. One earlier run was invalidated by a concurrent smoke test restarting its shared database; it was rerun sequentially and passed. No product check is claimed from that interrupted run.
- govulncheck module scan passes with no vulnerabilities. `golang.org/x/sys` was pinned to v0.44.0 to remove GO-2026-5024 even though it was Windows-only/unreachable. The working command is `go -C cmd/mailbox run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -scan=module`.
- Strict OpenSpec validation passes. SDK and transitive dependencies are locked in go.mod/go.sum.
- Docker build passes; additive fourth migration/readiness and previous foundation lifecycle checks pass.
- Real configured container enables MCP without browser identity, returns 401 for missing credentials and 403 for foreign/null Origin. Operations-only mode retains MCP 404.
- Two separately executed requester/responder binaries complete request/reply over actual HTTP SDK transport, with distinct user-owned credentials. Output contains IDs/status only, no tokens or message bodies.
- A real application binary is stopped with SIGTERM and restarted with a different PID; an offline message is read afterward over HTTP MCP. Application logs contain no token, database URL, or body sentinel.

## Requirement and Scenario Evidence

| Requirement | Evidence |
| --- | --- |
| Official authenticated remote transport | `TestMCPHTTPDifferentMembersSendReadAcknowledgeReplyRead` connects two SDK clients owned by different invited users and invokes all five tools over HTTP. Authentication boundary tests deny missing/invalid/revoked/cookie-only dispatch. |
| MCP Origin and body protection | `TestMCPHTTPCredentialOriginAndBodyBoundary` checks null/foreign/malformed/duplicate Origins on HTTP methods, valid native no-Origin calls, and chunked body overflow 413. `TestMCPHTTPPreservesSDKProtocolAndLocalhostProtection` checks SDK malformed-protocol 400 and rebinding Host 403. |
| Credential-derived identity and recipient directory | Separate HTTP whoami identities, own-workspace directory, foreign/inactive directory exclusion, and exact bearer-derived principals are checked. |
| Explicit mailbox tool schemas | HTTP tools/list publishes five object schemas with additionalProperties=false. Native calls reject sender/workspace/inbox/whoami identity substitution. |
| Tool limits | Unit boundary tests cover text/UTF-8/NUL, metadata bytes/depth, UUID/key/kind limits. Native tests reject invalid fields/null/limits and measure the actual serialized SDK result at or below 256 KiB with continuation. |
| Stable application failures | Native tests inspect `invalid_argument`, `not_found`, and `idempotency_conflict`; current authorization/database errors map to bounded documented envelopes. SDK protocol errors remain SDK behavior. No private payload is echoed. |
| Executable two-client request reply | `TestMCPExecutableRequesterAndResponderProcesses` builds and starts two separate OS processes, with distinct credentials and bounded polling, and verifies requester/responder completion without token/body logging. `docs/mcp.md` explains no automatic Codex/Claude wakeup. |
| Committed durable messages | Send/read/ack/reply tests assert stored IDs/content. Failed message insert rolls back thread/counter/message. Offline messages survive HTTP-server reconstruction and a real binary process restart. |
| Credential-scoped sender and eligible recipient | Different owners communicate in one team; native/direct tests deny inactive, foreign and removed-member recipients, spoofed sender/workspace arguments, and preserve actual sender/workspace. |
| Participant-scoped threads and replies | Valid replies retain thread/reply IDs; tests deny foreign-workspace, invisible third-party, wrong participant-pair and inconsistent thread/reply references without linked data. |
| Sender-scoped idempotency | Eight concurrent service retries and two concurrent independent HTTP clients produce one keyed message. Changed payload conflicts; canonical metadata/defaults and rotated-token retries return the original message; committed retry survives recipient deactivation. |
| Metadata fidelity and external-data semantics | Native large integers beyond 2^53 remain exact; object/numeric formatting/default equivalence is tested. Compact exponents remain exact in PostgreSQL JSON storage. SDK latest-protocol extraction rejects extreme exponents before dispatch; tests assert either exact success or clear rejection with zero persistence, never rounding. Tool descriptions/docs treat content as external data. |
| Explicit retention | No deletion job/code exists. Acknowledged data remains readable with all/acknowledged filters and keyed sends remain queryable. Documentation states indefinite MVP retention and separately approved future pruning. |
| Own inbox and nondestructive reads | Repeated reads return unacknowledged data unchanged; other agents' inputs/cursors and nonrecipient reads never expose another inbox, including an administrator's sender identity. |
| Idempotent own acknowledgement | Recipient acknowledgement repeats with exactly the original non-null timestamp; sender/third-party acknowledgement returns not_found and retains data. |
| Commit-ordered inbox sequences | `TestMailboxCounterWaitsForEarlierCommit` holds a real SQL advisory gate in the first insert, observes a later send waiting on the inbox counter, then checks sequence 1/2 and pagination without loss. |
| Signed snapshot pagination | An insert between pages is excluded from the old snapshot and available on fresh polling. Signer reconstruction keeps valid cursors; tampering, wrong key/purpose/agent/workspace/filters are denied. Count and actual wrapper byte limits both hold. |
| Live acknowledgement filtering | Another client acknowledges an unseen page item; continuation omits it from unacknowledged data and acknowledged filtering still retrieves it. |
| Shared identity and repeated processing | Two independent HTTP clients using one agent credential read the same messages; no lease/claim or exactly-once execution is implemented or advertised. |
| Modified runtime availability | Real configured container and process expose authenticated MCP independently of browser login; absent mailbox settings retain 404. Identity and health behavior remain covered by earlier tests. |

## External Limits

SDK v1.8.0 latest-protocol header extraction can reject extreme numeric exponents outside an intermediate float range; accepted values remain exact and the limitation is documented. This project uses the official unmodified transport.

Live GitHub OAuth, Codex/Claude model-driven sessions, hosted GitHub Actions, paid provisioning, and Railway deployment have not been claimed from these local tests. Codex/Claude configuration checks, final management UI, full runbooks and release/CI readiness are change 5. No arbitrary agent process is started by the server.
