# GitHub repository admission

`internal/repositoryaccess` checks owner or explicit direct-collaborator identity.
It does not grant access because a repository is public. The enrollment service
must supply the authenticated immutable GitHub user ID and a trusted project
mapping containing repository ID, owner ID, owner login and repository name.
Repository replacement, transfer or mismatched identity fails closed.

## Server configuration

Use a server-side GitHub integration token source with access to the configured
repository. Prefer a GitHub App installation token or a fine-grained token with
repository Metadata read permission and the access required by the collaborator
endpoint. Classic/OAuth tokens require the documented repo/read:org scopes and
repository privileges. Never give this integration token to models or runners.
The existing OAuth client secret is not a REST API access token.

A wiring example is `repositoryaccess.New(tokenSource)`, where tokenSource
retrieves a current token from the private server configuration or installation
refresh service. No environment variable is consumed by this package itself;
actual enrollment/configuration wiring is tracked in OpenSpec task 3.2.

`Verify(ctx, repository, githubUserID, force)` fetches repository metadata and
checks IDs before admission. The owner is admitted by immutable owner ID. Other
users must appear in the paginated collaborators response with
`affiliation=direct`; usernames supplied by clients are not authority. Every
request uses the fixed GitHub API origin, bounded responses and a 5-second
technical context. Redirects, inaccessible/rate-limited APIs, malformed or
oversized data and incomplete pagination fail closed with safe errors.

## Cache and refresh

Cache age is at most 60 seconds measured from verification start, including
negative membership results. At expiry, removal becomes a denial or unavailable
verification. Startup/credential refresh must pass force=true. Failed forced
refresh invalidates cached admission. Concurrent older requests cannot overwrite
newer authority results. The cache is bounded to 10,000 entries.

An unexpired cache is intentionally usable for its documented window; no stale
success survives an attempted failed refresh. Network deadlines bound technical
verification and do not create task execution deadlines.

## Verification

Run `go test -race ./internal/repositoryaccess`. Tests cover owner/direct member,
public-reader denial, repository ID mismatch, pagination, 60-second removal,
forced-refresh outage, HTTP failures/body bounds and concurrent stale checks.
These tests use a fake GitHub transport and do not claim live enrollment or
provisioned production integration credentials.

API behavior and required permissions follow the official
[GitHub collaborator documentation](https://docs.github.com/en/rest/collaborators/collaborators#list-repository-collaborators).

## Selected deployment integration

The current server integration uses GitHub App installation tokens rather than
GITHUB_REPOSITORY_TOKEN. Configure GITHUB_APP_CLIENT_ID,
GITHUB_APP_INSTALLATION_ID and GITHUB_APP_PRIVATE_KEY server-side. The token
provider requests Metadata read only, signs short-lived App JWTs, shares cached
installation tokens and renews before expiry. An unsuccessful refresh returns
no stale token. The App must be installed on the repositories the platform
serves; this setup does not grant individual runners additional roles.
