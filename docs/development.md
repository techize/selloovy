# Local development

This increment serves a Go-rendered storefront preview, Vue admin preview and PostgreSQL readiness. Operator setup and owner sign-in are available when configured. Authenticated shop identity settings are editable. Private product drafts, variants and stock are editable. Storefront catalogue, jobs and payments are not implemented.

## Toolchain and startup

Use Go 1.26.5 and Node 26.5.0, pinned in go.mod/.node-version and CI. PostgreSQL 17 is the current database baseline (CI pins 17.11). pgx/Tern and frontend dependencies are pinned in go.mod/package-lock.json. sqlc generates authentication, shop and catalogue queries.

```sh
go mod download
npm --prefix web/admin ci --ignore-scripts
npm --prefix web/admin run typecheck
npm --prefix web/admin run build
go run ./cmd/selloovy
```

Visit http://127.0.0.1:8080. Stop with Ctrl-C. To test worker startup/shutdown separately, run `go run ./cmd/worker`; it waits for a stop signal and does not process jobs yet.

Visit http://127.0.0.1:8080/admin/ for the Vue preview. Navigation changes the displayed preview section; it does not access merchant data. Without authentication configuration this remains a public preview. Configured installations show the owner sign-in form; the protected workspace endpoint verifies a server-side session. The connection card reads only the existing public health endpoint. This is not an operating merchant dashboard.

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

The server sets header/read/write/idle timeouts. Health responses disclose only status and are not cached. No request body, payment token or customer details are logged. Payment routes are absent; shop identity editing is authenticated.

## Owner setup and browser sign-in

First migrate the database and build the frontend. Provision one private installation key; do not regenerate it after enrolling an owner. The following example uses ignored local files and the explicit loopback development exception:

```sh
source local/database.env
go run ./cmd/migrate
go run ./cmd/auth-keygen local/auth.key
export SELLOOVY_AUTH_KEY_FILE=local/auth.key
export SELLOOVY_PUBLIC_ORIGIN=http://127.0.0.1:8080
go run ./cmd/owner-setup
go run ./cmd/selloovy
```

Run owner-setup yourself in an interactive terminal. It asks for your email, hidden password and shop name, then asks whether to enable MFA now. Press Enter or choose n to skip it and sign in with your password. Admin shows a persistent recommendation and an Enable MFA button for later QR setup. Choosing y in the CLI instead displays a private enrollment key (SHA1, six digits, 30 seconds); confirm its current code and save the ten recovery codes privately. Do not paste credentials into chat, shell arguments or Git. Disable terminal recording/sharing before setup. Interactive prompts have no overall deadline; each database operation has its own thirty-second timeout. If interrupted before confirmation, rerun with the same email/password to resume. It cannot replace an enrolled owner's MFA. There is no public registration or setup HTTP route and no default owner/password.

The key file must be a regular non-symlink file, 32 binary bytes, readable only by its owner. Key generation never overwrites an existing file. Back it up separately from the database and restrict filesystem access. Losing the key prevents MFA decryption; rotation and restore procedures remain live-readiness work. Kubernetes projected-secret symlinks are not accepted by this loader; a future deployment must provision a private regular file securely.

Use the exact configured origin in the browser. When MFA is off, password verification grants a session. When enabled, password verification advances to MFA; an authenticator or unused recovery code then grants a session. Recovery still requires the password and leaves MFA enabled. Sign out revokes the current browser session and any presented pending challenge. The current workspace remains a development preview with no commerce operations. `/api/auth/workspace` demonstrates server-enforced owner access; rendering the Vue shell grants no permissions.

Both authentication variables and a database are required together; invalid configuration fails startup. Without them, auth routes are absent and the public shell remains accessible. Outside a loopback-bound development process, configure an HTTPS origin and terminate TLS securely at the trusted ingress. Preserve the configured Host and Origin; this increment ignores forwarded client IPs, so proxy connections share an IP quota. Do not expose this login publicly until the remaining controls in [owner authentication delivery](owner-authentication.md) are verified. No live deployment is configured here.

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

## Authentication query generation

Install the pinned generator with `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1`
and ensure the Go binary directory is on PATH. Run `sqlc generate` after changing
schema/query source, then review the generated internal/authdb, internal/shopdb and internal/catalogdb diffs. CI regenerates
and rejects drift. sqlc/its build dependencies are developer tools, not bundled
application runtime modules. Schema version 9 is an explicit forward migration;
an older development database must be migrated before the new binary is ready.
No default owner or encryption key is created. Authentication store tests use
uniquely named disposable databases and generated synthetic credentials only.


## Enable MFA later in admin

Use the persistent security recommendation to open setup, enter your current
password and scan the private QR code. Enter an authenticator code to enable MFA;
unconfirmed or cancelled setup leaves password login available. Save the ten
recovery codes, then sign in again with a fresh code. Activation signs out all
prior sessions. Enabled MFA cannot be bypassed by rerunning CLI setup.


## Shop identity settings

After sign-in, open Settings to edit your shop name, tagline, description and
customer contact email. Save persists these in PostgreSQL; it does not publish
them to the public storefront. A stale browser tab must reload saved details
before another save. UK defaults are currently read-only. See shop-settings.md
for API limits, ownership and subsequent VAT/shipping/publishing work.

## Product drafts

After explicit migration to schema 6 and frontend rebuild, sign in and open Products.
Add a draft, enter a final GBP price and choose its certificate name requirement.
Saved products stay private; see [product draft delivery](product-drafts.md).
Existing shop settings and owner credentials are preserved by the migration.

## Maker variants and stock

After explicit migration to schema 7 and frontend rebuild, open Products and choose
Variants & stock. Add each size/colour combination and its count. The product’s
fallback checkbox controls zero-stock production. Blank override prices inherit
the product price. Counts are manual; checkout holds and sales deductions follow
later. See [maker variant delivery](maker-variants.md) and Go lesson 10.

## Reviewed public catalogue

After explicit migration to schema 8 and frontend rebuild, open Products →
Publish & preview. Review the shop and product, then publish. Follow the generated
public link to browse its variant prices and availability. Republish updates
reviewed content; Unpublish removes the product. See storefront-publication.md.
Cover photos are now available separately. Guest basket and certificate owner-name collection are available; checkout remains open.

## Product cover photos

Apply the current schema explicitly and rebuild admin. Products → Photo uploads a bounded
JPEG/PNG with required alt text, or edits/removes its draft cover. Publish & preview
includes the saved photo. Republish applies photo changes to the public catalogue.
Images are stored in the database and included in its backups. No new environment
variable or separate upload directory is required. See product-photos.md.


## Guest basket

Apply the current schema explicitly and restart the Go app. Configure the existing public
origin to enable basket forms. Open a published product, choose its option and
quantity, enter a certificate owner name where offered, then Add to basket.
Update quantities/names or remove lines on Your basket. Data survives app restarts
and expires after seven days. Stock holds, checkout, payments and admin
abandoned-basket views follow later. See [guest-basket.md](guest-basket.md).


## Shipping and preparation

Apply schema 11 and restart the Go app; rebuild admin. Settings → Shipping →
Use UK starter services creates an editable draft, then Save shipping services
applies it. Existing shops start with no services, so no charges are silently
enabled by migration. Products → Variants & stock edits made-to-order preparation.
A basket can select delivery, see product-plus-shipping totals and date previews.

Default England and Wales bank holidays cover 2026–2028. A reviewed GOV.UK feed
file can override them via SELLOOVY_BANK_HOLIDAY_FILE; restart to load it. Invalid
files stop startup and dates outside coverage are not guessed. See shipping.md.
