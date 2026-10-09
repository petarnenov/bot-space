# Railway Deployment Preparation

`railway.toml` and the Dockerfile define deployment settings. The owner authorized
Railway publication on 2026-10-09; the [live deployment record](live-deployment.md)
tracks executed checks and the remaining GitHub activation. Historical MVP
verification reports describe the earlier preparation-only stage.

The Docker build uses Go 1.27.2 and a digest-pinned non-root Distroless runtime with
CA certificates. The same `/mailbox` binary serves HTTP and runs migrations.
The application listens on `0.0.0.0:$PORT` and reads PostgreSQL `DATABASE_URL`.
Durable state belongs to PostgreSQL, not the ephemeral application filesystem.

## Deployment Settings

Configure a Railway PostgreSQL connection and inject `DATABASE_URL` as a secret.
Keep application secrets in Railway environment variables. Use one replica for
the MVP; application rate limits are per replica, not distributed guarantees.

The pre-deploy command is `/mailbox migrate`, with a 330-second Railway timeout
covering the application's five-minute total migration deadline. Railway runs
pre-deploy in a separate container; filesystem changes are not persisted.
Migration failure prevents the new deployment from proceeding.

`/mailbox serve` starts the service. `/readyz` is the deployment healthcheck with
a 300-second startup allowance. Railway injects `PORT`; do not hardcode the host
port. Railway's healthcheck hostname is `healthcheck.railway.app`; health routes
have no Host restriction or rate quota. Railway checks deployment
readiness, not continuous uptime after deployment.

## Production Setup

Choose the production domain and require TLS. Register the GitHub OAuth app with
the exact callback `https://YOUR_DOMAIN/auth/github/callback`. Local callback
configuration is `http://localhost:8080/auth/github/callback`. Set
`GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and `PUBLIC_BASE_URL` to that app's
credentials and the exact HTTPS service origin. Request no repository permissions.
Set a stable `CURSOR_SIGNING_KEY` (32 random bytes as 64 hexadecimal characters)
and, if needed, exact comma-separated `MCP_ALLOWED_ORIGINS`. Rotate secrets via
environment configuration; changing the cursor key invalidates existing cursors.

After migrations, run the administrative command once with your immutable GitHub
ID: `/mailbox bootstrap-owner --github-user-id YOUR_ID --workspace YOUR_SLUG`.
Follow [identity setup](identity.md), sign in, then invite the second account and
issue each user's own agent credential. Repeat the [two-client exchange](mcp.md)
over the final HTTPS domain before declaring a deployment operational.

Browser sessions must use secure cookies in production. Agent credentials must
travel over TLS. Configure trusted origins without wildcard credentialed CORS.
Confirm [database backups and isolated restore](database.md) before rollout.
Do not publish example credentials, database URLs, or real message content.

## Verified Boundaries and Remaining Live Checks

The current official JSON schema accepts `railway.toml`. Local tests verify
non-root container serving/migration, injected port, readiness, and the unchanged
SDK Host policy. A direct localhost connection accepts localhost Host; a public
Host on a loopback connection is denied as DNS rebinding protection. Simulated
nonloopback proxy connections permit a public Host after bearer authentication.
Forwarding headers do not alter Host protection or peer rate keys.

Do not disable SDK protection to work around a loopback proxy. The real Railway
proxy path, TLS/domain, production OAuth, and managed backups remain unverified
until an explicitly authorized deployment. Confirm the connection topology and
public `/mcp` exchange then. Pre-deploy/start commands execute the binary directly;
the Distroless image has no shell. `PORT` is read by Go rather than shell expansion.

## Sources

- [Official configuration schema](https://railway.com/railway.schema.json)
- [Dockerfile builds](https://docs.railway.com/builds/dockerfiles)
- [Deployment healthchecks](https://docs.railway.com/deployments/healthchecks)
- [Pre-deploy commands](https://docs.railway.com/deployments/pre-deploy-command)
