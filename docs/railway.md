# Railway Deployment Preparation

`railway.toml` and the Dockerfile prepare deployment. No Railway resources have
been created and no live Railway deployment has been tested. Paid provisioning
and publication require explicit owner permission.

The Docker build uses Go 1.27.2 and a digest-pinned non-root Distroless runtime with
CA certificates. The same `/mailbox` binary serves HTTP and runs migrations.
The application listens on `0.0.0.0:$PORT` and reads PostgreSQL `DATABASE_URL`.
Durable state belongs to PostgreSQL, not the ephemeral application filesystem.

## Deployment Settings

Configure a Railway PostgreSQL connection and inject `DATABASE_URL` as a secret.
Keep application secrets in Railway environment variables. Use one replica for
the MVP; later application rate limits are per replica until otherwise specified.

The pre-deploy command is `/mailbox migrate`, with a 330-second Railway timeout
covering the application's five-minute total migration deadline. Railway runs
pre-deploy in a separate container; filesystem changes are not persisted.
Migration failure prevents the new deployment from proceeding.

`/mailbox serve` starts the service. `/readyz` is the deployment healthcheck with
a 300-second startup allowance. Railway injects `PORT`; do not hardcode the host
port. Railway's healthcheck hostname is `healthcheck.railway.app`. Later host
validation must permit that host on health routes. Railway checks deployment
readiness, not continuous uptime after deployment.

## Production Setup for Later Changes

Choose the production domain and require TLS. Register the GitHub OAuth app with
the exact callback `https://YOUR_DOMAIN/auth/github/callback`. Local callback
configuration is `http://localhost:8080/auth/github/callback`. GitHub OAuth client
ID/secret and a configured public base URL arrive with the identity change; these
features are not implemented by foundation. Request no repository permissions.

Browser sessions must use secure cookies in production. Agent credentials must
travel over TLS. Configure trusted origins without wildcard credentialed CORS.
Confirm [database backups and isolated restore](database.md) before rollout.
Do not publish example credentials, database URLs, or real message content.

## Sources

- [Official configuration schema](https://railway.com/railway.schema.json)
- [Dockerfile builds](https://docs.railway.com/builds/dockerfiles)
- [Deployment healthchecks](https://docs.railway.com/deployments/healthchecks)
- [Pre-deploy commands](https://docs.railway.com/deployments/pre-deploy-command)
