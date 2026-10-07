# Local development

This increment serves a Go-rendered storefront preview, Vue admin preview and PostgreSQL readiness. Merchant functions, authentication, jobs and payments are not implemented.

## Toolchain and startup

Use Go 1.26.5 and Node 26.5.0, pinned in go.mod/.node-version and CI. PostgreSQL 17 is the current database baseline (CI pins 17.11). pgx/Tern and frontend dependencies are pinned in go.mod/package-lock.json. sqlc-backed feature queries will follow.

```sh
go mod download
npm --prefix web/admin ci --ignore-scripts
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
go run ./cmd/selloovy
```

Visit http://127.0.0.1:8080. Stop with Ctrl-C. To test worker startup/shutdown separately, run `go run ./cmd/worker`; it waits for a stop signal and does not process jobs yet.

Visit http://127.0.0.1:8080/admin/ for the Vue preview. Navigation changes the displayed preview section; it does not access merchant data. Login/MFA and privileged APIs are absent. The connection card reads only the existing public health endpoint. This is not an operating merchant dashboard.

Vite writes ignored web/admin/dist assets; Go serves them on the same origin. Node is a build dependency, not an application server. Run frontend build before packaging the Go binary, and ship the built directory with it. `SELLOOVY_ADMIN_DIR` can select that trusted build directory; it must not point at uploads, source code or private data. Missing index returns 503. Only the index and regular files beneath assets/ are served; directory listing and other admin paths are unavailable. Rebuild/reload after frontend changes. Source maps are disabled. Never place credentials in any frontend variable: `SELLOOVY_PUBLIC_` variables are explicitly public build inputs, not secret storage.

For process-level signal checks, build and run the binary directly:

```sh
go build -o bin/selloovy ./cmd/selloovy
./bin/selloovy
```

The default binding is loopback only. Set `SELLOOVY_HTTP_ADDR` to an IP address and port to override it, for example `127.0.0.1:9000` or `[::1]:8080`. Setting a non-loopback address exposes the preview on that interface; use an isolated development environment. Invalid or explicitly empty values fail startup. Configuration values are not printed in startup errors.

No configuration file is read automatically. Credentials belong in ignored local files or an external secret store, never committed files. The dedicated database helper below generates its own ignored development password; no paid account is required.

## Dedicated local PostgreSQL

With PostgreSQL tools and OpenSSL installed, the helper creates its own data directory under ignored `local/`, generates a local password, and binds only loopback port 55432. It does not start or modify a system-wide service. Check that this port is free first. It creates development and test databases; the local account has elevated privileges only for this isolated development cluster.

```sh
bash scripts/dev-postgres.sh start
source local/database.env
go run ./cmd/migrate
go run ./cmd/selloovy
```

Do not print, copy or commit local/database.env or its password file. The app reads the environment; `source` explicitly configures the current shell. Migration is a separate forward-only command with a bounded timeout. Application startup never changes the schema. Repeating the migration is safe when the schema is already current. Never edit an applied migration; add a new numbered file and update the expected version.

To stop the dedicated database, run `bash scripts/dev-postgres.sh stop`. Restart with `start`; data is retained. Do not delete `local/` unless intentionally discarding the development database and credentials. Its logs/configuration are private and ignored. Use distinct migration/runtime roles and externally managed credentials for a future live deployment; the development superuser is not a production permission model.

`SELLOOVY_DATABASE_URL` requires an explicit PostgreSQL URL with host, user and database. Use `sslmode=disable` only with an explicit loopback IP; other connections require `sslmode=verify-full`. Driver/configuration errors are redacted. Without the variable the preview still starts but readiness is unavailable. A blank or invalid variable fails startup.

## Health and lifecycle

- `GET /health/live`: 200, confirms the HTTP process responds independently of persistence.
- `GET /health/ready`: 200 only when the configured database responds with the exact shipped schema version; otherwise 503. Missing, unapplied and newer schemas remain unready. This checks the foundation dependency, not overall commerce readiness.
- SIGINT/SIGTERM: stop accepting connections and allow up to ten seconds for in-flight requests, then force-close if necessary.

The server sets header/read/write/idle timeouts. Health responses disclose only status and are not cached. No request body, payment token or customer details are logged. Privileged admin and payment routes are absent.

Credential helpers and their tests now exist in internal/auth, but no sign-in is wired up. See [owner authentication delivery](owner-authentication.md) for the implemented layer and remaining persistence/session/recovery gates. The preview requires no encryption key or owner credentials.

## Verification

```sh
go mod verify
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
go vet ./...
go test -race ./...
go build ./cmd/...
go run ./scripts/publicguard.go
gitleaks git --log-opts=--all --redact --no-banner --ignore-gitleaks-allow
```

Check formatting with `gofmt -l cmd internal scripts db` (no output means formatted). Tests cover configuration redaction, liveness independence, dependency readiness failures without disclosure, route boundaries and graceful completion of an in-flight request.

With `source local/database.env`, the test URL is configured too. `go test -race ./...` then creates and removes a uniquely named database for integration verification. It tests repeated migrations, saved data across pool reconnects, future-schema refusal, and database outage health behaviour. The test account needs database creation privileges. Only use a dedicated `selloovy_test` database; no real data. If the test variable is absent, integration tests explicitly skip; that is not a database verification pass. CI always supplies its isolated test database.
