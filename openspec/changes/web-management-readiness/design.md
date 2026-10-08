# Design

## Context

The first four changes are verified and archived. Existing services enforce browser identity/CSRF, workspace roles/invitations, agent ownership/credentials, and private SDK mailbox delivery. The public GitHub repository already contains foundation commit `6def6f7`, matching local history. The user requested that public repository and its CI and supplied owner/name; source publication is within that deliverable. Railway publication/paid provisioning remains unauthorized.

Codex CLI 0.162.0, Claude Code 2.1.292 and GitHub CLI are installed. Official OpenAI docs identify `bearer_token_env_var`; local commands and isolated temporary config will verify syntax without altering the user's settings. Actual Go SDK wire exchange is already proven. Do not launch model-driven agents merely to claim interoperability; distinguish each tested scope.

## Goals / Non-Goals

**Goals:** all requested management pages, admission limits, complete browser/MCP acceptance, client records, backup/restore, fresh checkout and hosted CI proof, and final requirement audit.

**Non-Goals:** another frontend framework, admin inbox UI, live OAuth registration, arbitrary runners, cloud/paid provisioning, or Railway deployment without new permission.

## Decisions

### Server-rendered management

Add an `internal/web` management layer using existing identity protection and core services. Allow identity Web to delegate home/workspace rendering, preserving its login/callback/logout behavior. Embed a small CSS asset; use standard HTML forms and accessible labels/tables. Header policies stay no-store/no-referrer/CSP/nosniff. No separate frontend build is introduced.

Workspace pages query the caller's current membership, show member/role metadata and appropriate forms, owned agent/credential metadata, and owner/admin team-agent deactivation. Server handlers always derive actor from Session and recheck core policy. UI visibility is only presentation, never authorization. Owners alone see owner-grant choices; last-owner protection remains transactional. There are no inbox/message-content views.

Invitation creation returns a one-time result/link and expiry; listings never contain secrets. GET acceptance only renders a form. A short-lived HttpOnly/SameSite/Secure-in-production pending-invitation cookie can carry the user's already-present link secret through login without persisting plaintext in server attempt/session/database records. Return path contains only the acceptance route, never a secret. Explicit same-origin/CSRF POST performs identity-bound acceptance, then expires the pending cookie. Attacker-controlled cookie/link values grant nothing without the real secret and target identity. Logs never include query/cookies.

Reuse existing agent POST handlers and one-time result pages; add full forms and metadata lists. Store.RawToken remains only in the issuance/rotation response. Share a small embedded stylesheet with result pages and default identity pages for consistent readable navigation.

### Request admission

Implement a bounded, mutex-protected token bucket keyed by peer IP, with injectable clock for deterministic tests. Limit state to 10000 keys; reclaim idle entries after ten minutes; deny new keys when full. Global route admission wraps the existing HTTP Handler before eager body buffering: login 20/min burst 5, MCP 120/min burst 60. Health endpoints bypass these buckets. Do not trust forwarding headers from callers.

Inside MCP, authenticate first, then apply an aggregate per-agent 60/min burst-20 bucket before SDK dispatch, shared by all its credentials/clients. Return 429 with a safe `rate_limited` JSON envelope and Retry-After, without tool work. State is per replica; keep Railway numReplicas=1. Actual proxy peers can share a bucket; document that behavior rather than claiming an unverified public-client IP policy. No Redis is added.

### Proxy and deployment preparation

Review current Railway start-command semantics: Docker start command overrides ENTRYPOINT in exec form, so `/mailbox serve` is valid and PORT is read by Go without shell expansion. Retain explicit `/mailbox migrate` predeploy, bounded timeout, `/readyz`, non-root image, pinned dependencies/images, and no durable app filesystem.

Preserve SDK rebinding defaults for direct requests. If a trusted loopback proxy simulation requires a public Host, add a strict configured-host equivalent policy and verify rejection of foreign hosts before changing SDK's localhost option. This does not prove a live Railway route.

Run an actual local pg_dump/pg_restore drill into a separate database with synthetic state. Verify migrations/checksums, messages, idempotency and revoked/active credential state after restore. Destroy only the disposable restore target and keep dumps outside the repository. Document production backup ownership/retention settings as operator-provided.

### End-to-end and client verification

Extend real-PostgreSQL mock-GitHub acceptance tests to interact with actual forms/routes for two accounts, invite/accept, registration/issuance, and two independent SDK connections. Test UI authorization/CSRF, forwarded invitation denial, admin inbox privacy, string escaping, rate admission/memory and aggregate-agent quotas. Keep previous direct domain tests as additional evidence.

Use temporary isolated client config locations for Codex and Claude syntax/connection checks, never changing user config or storing real secrets in public examples. Prefer official docs and actual CLI help over guessed commands. Go client/session and two-process demo already prove model-neutral HTTP behavior. A model-assisted tool run is a different, unclaimed scope unless actually executed with authorization.

### Release evidence

Run local checks and complete a clean-checkout reproduction after committing the verified code. Review tracked files for real secrets/data and AGENTS.md changes. Update the existing public repo without overwriting divergent remote work; first fetch and confirm ancestry. Run/monitor its actual GitHub Actions workflow and tie green status to the source SHA. No paid deployment follows from source publication.

Maintain a final original-brief requirement matrix with explicit evidence and unexecuted external limits. Only sync/archive when every tracked task passes; publish final archive/evidence commits and verify their CI as well.

## Risks / Trade-offs

- [Per-replica/actual-peer limits] → explicit one-replica and shared-proxy semantics; distributed/public-IP limits need a verified policy later.
- [Invitation link in browser state] → short-lived protected client cookie, no server plaintext persistence, no-referrer/no-store, explicit targeted POST.
- [CLI configuration versus model execution] → record actual evidence level and avoid claiming model-driven exchange.
- [Hosted CI differences] → inspect exact failing job/commit and fix real failures, without narrowing checks.
- [Live external setup] → prepare domain/OAuth/secrets/runbooks; do not claim or perform unauthorized Railway deployment.

## Migration Plan

No new domain migration is required. UI and limiter changes reuse the four versioned migrations. Rebuild and run the local service, verify all previous behavior plus final acceptance, then publish source/CI evidence to the existing requested public repo. Preserve operator secrets/volumes and all existing AGENTS.md bytes.

## Sources

[OpenAI MCP configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli), [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference), [Railway start commands](https://docs.railway.com/deployments/start-command), [Railway config reference](https://docs.railway.com/config-as-code/reference), and installed Codex/Claude CLI help checked on 2026-10-09. Fetch official Claude guidance before final examples; all client/version claims require actual evidence.
