# Native gRPC control transport

The typed contract is `api/control/v1/control.proto`; generated Go messages and
client/server stubs are committed. `internal/control.New` registers the native
service with mandatory authentication and backend dependencies. TLS credentials
must be supplied for a direct public listener; an ingress TLS deployment needs
separate verification of the upstream HTTP/2 path. No public endpoint is yet
verified or enabled by this package.

## Generate

Use protoc 25.0 and Go 1.27.2. Install pinned plugins in a task-specific bin
folder, add that folder to PATH, then generate from the repository root:

```sh
GOBIN=/absolute/tool-bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
GOBIN=/absolute/tool-bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
PATH=/absolute/tool-bin:$PATH protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/control/v1/control.proto
```

## Behavior and bounds

Presence and Inspect are unary. Connect starts with a Resume cursor and then
exchanges typed votes, proposals, questions, progress, results and ACKs alongside
independently delivered assignments, council updates and answers. Send and
receive operate concurrently, without an application transport queue.

Messages are limited to 256 KiB and each connection to 32 concurrent streams.
Authentication, unary/backend mutations and the opening handshake have a
5-second technical bound. The established stream and durable work have no
execution or discussion duration cap. Native flow control supplies backpressure;
backend Pull must honor cancellation and return durable events in cursor order.

The authenticator rechecks runner/project/role/credential epoch before every
incoming mutation and outgoing event. Open-stream revocation and role changes
stop privileged traffic. Executor-only and architect-only frames are separated.
Backend policy must additionally validate project resource ownership, revisions,
reservations and evidence. Backend errors are reduced to safe status messages;
request bodies, credentials and backend diagnostics are not logged here.

Sending advances only the connection's delivery cursor. It does not persist an
ACK or prove execution. The backend owns durable event storage; the runner must
persist before ACK. Cursor replay/storage and GitHub collaborator enrollment
remain separate tracked implementation tasks.

## Verification

`go test -race ./internal/control` exercises native TLS unary/trailers, incoming
and outgoing size bounds, independent bidirectional traffic, reconnect cursor,
role rejection and revocation in both directions using an explicit in-memory
backend fixture. It does not prove durable storage or Railway ingress support.
Follow the official [gRPC Go generation guide](https://grpc.io/docs/languages/go/quickstart/).

## Public ingress evidence (2026-10-09)

The native Go client failed through `bot-space-production.up.railway.app:443`
on deployment `cf343487-7f56-4aed-98e4-d3c9e27bbbb8`. The edge returned HTTP
505; Railway request logs identified downstream HTTP/2.0 and upstream HTTP/1.1
to application port 8080. Local h2c tests passed, so public edge HTTP/2 alone
does not provide native gRPC transport to this listener. Unary authentication,
trailers and bidirectional public behavior remain unverified. Existing health,
readiness, GitHub login and unauthenticated MCP returned 200, 200, 302 and 401.

The diagnostic token is removed after this unsuccessful probe. Its synthetic
transport events do not prove persistence or operational enrollment. The next
transport candidate is a separate TLS gRPC listener through Railway's TCP
proxy; it must be implemented and tested before replacing this failed route.
The browser/MCP HTTPS address remains unchanged. Do not silently substitute
gRPC-Web, WebSocket or an unverified HTTP gateway for native gRPC.

The HTTP/2 dependency is pinned to `golang.org/x/net v0.60.0`, fixing the five
module vulnerabilities reported by the probe CI run. Module vulnerability scan
and affected race tests pass after the update; hosted CI must also pass.
