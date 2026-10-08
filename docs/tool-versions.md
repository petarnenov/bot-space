# Tool Versions

Verified on 2026-10-08. Versions listed as targets are not installed or runtime
verified merely by being listed here.

| Tool | Observed locally | Approved project target |
| --- | --- | --- |
| Go | 1.21.4 system; 1.27.2 verified task toolchain | 1.27.2 |
| OpenSpec | 1.14.1 | 1.14.1 |
| Node.js (OpenSpec only) | 24.21.0 | 24.21.0 |
| npm | 11.19.0 | 11.19.0 |
| Git | 2.33.0 | compatible Git |
| Docker CLI/Engine | 29.5.2 | compatible Docker Engine |
| Docker Compose | 5.1.3 | Compose with service completion dependencies |
| PostgreSQL | 18.6 in Docker | 18.6 |
| pgx | v5.11.0 locked | v5.11.0 |
| Official MCP Go SDK | introduced in change 4 | v1.8.0 |

The Go download API was rechecked and lists Go 1.27.2 as stable. The approved
design records official release and documentation sources. Container digests,
scanner versions, and action SHAs will be recorded when verified. No Railway
deployment or hosted GitHub Actions run has been performed.

Additional verified pins:

- `golang.org/x/text` v0.41.0 replaces pgx's vulnerable transitive v0.29.0.
- `govulncheck` v1.8.0; Railway schema check tooling: jsonschema 4.25.1.
- Go build image: `golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`.
- PostgreSQL image: `postgres:18.6@sha256:74935e72241653ca55e0414067e6d8763aceb8a810eb51b452253ec3dcfc4336`.
- Runtime image: `gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3`.
- Actions checkout v5: `fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09`.
- Actions setup-go v6: `924ae3a1cded613372ab5595356fb5720e22ba16`.
- Actions setup-node v5: `a0853c24544627f65ddf259abe73b1d18a591444`.

The Go 1.27.2 darwin/amd64 archive SHA-256 was verified against the official
Go download API: `587b59182488b23aa6e5fc25110405a3e0e5b38ed2f5b2f46ed13c32aee356fe`.
It was extracted to a temporary task toolchain without replacing the system Go.
Registry manifests and action refs were inspected directly before pinning.
