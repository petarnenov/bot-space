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
Keep the flag off until those deployment gates pass. Registration/installation
and live App verification remain pending; unit tests use an explicit fake API.

See official [installation authentication](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation)
and [JWT requirements](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app).
