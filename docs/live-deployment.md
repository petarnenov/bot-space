# Railway Deployment Record

The owner authorized live deployment on 2026-10-09. Railway CLI 5.64.1 created
the bot-space project, application service, and PostgreSQL service with a durable
volume. No application filesystem is used for persistent state.

- Service origin: https://bot-space-production.up.railway.app
- MCP endpoint: https://bot-space-production.up.railway.app/mcp
- [Railway project](https://railway.com/project/39e8ccf7-4027-4795-ad70-c30d04af26fe)
- Application service ID: 8f6ff51b-1486-44c0-aa00-e7b83daae371
- Source: verified repository commit 65e2012549527a5dff0fd43ffc5ceb2c9d21bd9a

## Verified Initial Rollout

Deployment 80347bc9-5142-45a1-a66f-7f697963611d executed the migration command
and started the application. Public HTTPS health and readiness returned 200;
MCP without bearer credentials returned 401. The resolved deployment manifest
confirmed /mailbox serve, pre-deploy /mailbox migrate, a 330-second pre-deploy
timeout, readiness /readyz with a 300-second startup allowance, and one replica.

The first upload did not apply the deployment settings from railway.toml and
readiness correctly returned 503. The settings were explicitly written to the
service through Railway's API, then a fresh upload applied them. A redeploy of
the old deployment did not adopt the new settings. Verify the resolved manifest
and actual readiness after every configuration change; do not rely only on a
SUCCESS label.

Railway CLI currently reports Config as Code deprecated, with existing files
supported until 2026-12-01. Before that date migrate the authoring configuration
using the official CLI and review its plan; do not overwrite live secret values
or import them into tracked configuration. Current service settings are explicit.

## GitHub Activation Pending

The initial deployment enables the authenticated mailbox but does not yet enable
browser identity routes. The production OAuth credentials have not been supplied.
Health 200 does not prove the user-facing product is fully activated.

Create a GitHub OAuth App with homepage
https://bot-space-production.up.railway.app and exact callback
https://bot-space-production.up.railway.app/auth/github/callback.
Set GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET directly in the application service
Variables. Keep the secret out of chat, Git, screenshots and logs. Then set
PUBLIC_BASE_URL to the service origin and deploy the complete configuration.
The service requires the complete OAuth trio; partial configuration fails startup.

DATABASE_URL references the Railway PostgreSQL service. CURSOR_SIGNING_KEY was
generated securely and supplied through stdin; its value is stored only in
Railway variables. PORT is 8080 and DB_MAX_CONNS is 10. MCP_ALLOWED_ORIGINS
contains the exact public HTTPS origin. Do not rotate the cursor key unnecessarily.

The first owner is the immutable GitHub account ID 9674083 (petarnenov), with
workspace slug bot-space. Deployment 3fedf86a-ab2c-465c-97e1-23d2b6422854
successfully ran the one-time bootstrap after migrations, creating workspace
140826c3-f086-41ed-8530-99ad74de0922. Ordinary pre-deploy is restored to
/mailbox migrate afterward; future deploys do not repeat owner provisioning.
Distroless has no shell, so Railway SSH shell commands are unavailable. A temporary
deployment SSH key was registered for that check and then revoked and removed.

## Remaining Live Checks

Complete GitHub activation, confirm browser landing/login and secure cookie/redirect
behavior, and verify authenticated MCP through Railway's public proxy. Confirm
production backup retention/scheduling and an operator-managed restore drill.
Local synthetic restore and SDK exchange proofs remain in verification.md;
they do not substitute for these live checks.
