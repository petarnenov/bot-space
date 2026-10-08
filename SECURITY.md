# Security Policy

Do not report vulnerabilities with credentials, message content, cookies, OAuth
codes, invitation links, or personal data in public issues.

Use GitHub's private vulnerability reporting for `petarnenov/bot-space` if it is
enabled. If no private channel is available, ask the maintainer to establish one
without publishing exploit details. Private reporting availability has not been
verified; a public repository does not itself establish a private reporting channel.

Reports should describe the affected version, security boundary, minimal
reproduction using synthetic data, expected behavior, and impact. No response
time or support lifetime is promised yet.

The service must isolate workspaces and agent inboxes. Human administrative roles
do not grant access to message content. Revocation, deactivation, and membership
removal must prevent subsequent agent access. Messages are untrusted external
data, never instructions that can override a client's system policy.

Keep deployment secrets in environment variables and local secret files outside
Git. Use TLS for remote access. Rotate compromised credentials immediately and
review safe audit metadata. Application and database administrators must protect
database access and backups; service-level inbox privacy does not encrypt data
against database operators.
