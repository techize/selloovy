# Local development

This increment runs without a database. It serves a Go-rendered foundation preview, liveness and a deliberately unavailable readiness endpoint. No merchant admin, products, authentication, jobs or payments are implemented.

## Toolchain and startup

Use Go 1.26.5, pinned in go.mod and the CI preparation. Node is not required for this increment; Vue/TypeScript and their locked build dependencies will follow. PostgreSQL, pgx and sqlc setup remain pending the database task.

```sh
go mod download
go run ./cmd/selloovy
```

Visit http://127.0.0.1:8080. Stop with Ctrl-C. To test worker startup/shutdown separately, run `go run ./cmd/worker`; it waits for a stop signal and does not process jobs yet.

For process-level signal checks, build and run the binary directly:

```sh
go build -o bin/selloovy ./cmd/selloovy
./bin/selloovy
```

The default binding is loopback only. Set `SELLOOVY_HTTP_ADDR` to an IP address and port to override it, for example `127.0.0.1:9000` or `[::1]:8080`. Setting a non-loopback address exposes the preview on that interface; use an isolated development environment. Invalid or explicitly empty values fail startup. Configuration values are not printed in startup errors.

No configuration file is read automatically. Future credentials belong in ignored local files or an external secret store, never committed files. Nothing in this increment needs a secret or a paid account.

## Health and lifecycle

- `GET /health/live`: 200, confirms the HTTP process responds independently of persistence.
- `GET /health/ready`: 503 until a real persistence check is connected. Tests inject success/failure checks; they do not prove database availability.
- SIGINT/SIGTERM: stop accepting connections and allow up to ten seconds for in-flight requests, then force-close if necessary.

The server sets header/read/write/idle timeouts. Health responses disclose only status and are not cached. No request body, payment token or customer details are logged. Admin and payment routes are absent.

## Verification

```sh
go mod verify
go vet ./...
go test -race ./...
go build ./cmd/...
go run ./scripts/publicguard.go
gitleaks git --log-opts=--all --redact --no-banner --ignore-gitleaks-allow
```

Check formatting with `gofmt -l cmd internal scripts` (no output means formatted). The tests cover invalid configuration redaction, liveness independence, dependency readiness failures without error disclosure, route boundaries and graceful completion of an in-flight request.
