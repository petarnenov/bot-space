# The Firm GitHub App setup

This App verifies repository owner/direct-collaborator access. Human runner login
still uses the existing browser GitHub OAuth flow. Local Codex/Claude/Copilot
accounts remain independent.

1. Register a GitHub App in Developer settings → GitHub Apps. Choose an available
   English name such as `The Firm Bot Space` and homepage
   `https://bot-space-production.up.railway.app`.
2. For this verification integration, grant repository **Metadata: Read-only**.
   Disable webhook delivery while no webhook handler is configured. Additional
   write permissions are not required for identity verification.
3. Generate/download the App RSA private key. Keep the PEM private.
4. Install the App on the approved repositories, initially
   `petarnenov/bot-space`. Record the App Client ID and installation ID.
5. Set Railway service `bot-space` variables `GITHUB_APP_CLIENT_ID`,
   `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PRIVATE_KEY`. Supply the PEM using
   stdin or the private dashboard field, never chat or a command argument.

The code requests installation tokens with Metadata read only and renews them
before GitHub's one-hour expiration. No copied installation token is needed in
configuration. Existing `GITHUB_CLIENT_ID`/`GITHUB_CLIENT_SECRET` belong to browser
OAuth and are retained; do not replace them with App IDs by accident.

Automatic enrollment is gated by RUNNER_IDENTITY_ENABLED and also requires a
configured project registry/native control endpoint and matching TLS material.
Keep the flag off until those deployment gates pass. Registration, installation and live metadata verification are complete. Unit tests
use an explicit fake API; production enrollment activation remains pending.

See official [installation authentication](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation)
and [JWT requirements](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app).

## Verified installation (2026-10-09)

App: `The Firm Bot Space`, slug `the-firm-bot-space`, App ID `5249916`,
Client ID `Iv23liAVh5pRqpyjAQ5f`, installation ID `169580393`.
Installed on `petarnenov/bot-space` only, with Metadata read as its sole
repository permission and webhooks disabled. These identifiers are public;
the private key is never committed or printed.

The App variables were set privately on Railway service bot-space using stdin
and skip-deploys. A live Go probe acquired a metadata-only installation token,
verified immutable owner/repository IDs and received HTTP 200 from the direct
collaborators endpoint (one current member). No token was disclosed. The probe
source was removed after verification. RUNNER_IDENTITY_ENABLED remains disabled
until the project registry/native authenticated service activation gates pass.
