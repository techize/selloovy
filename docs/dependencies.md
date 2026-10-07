# Dependency record

Reviewed 7 October 2026. Review new dependencies and transitive rights as each feature is added; this record covers the current Go/PostgreSQL foundation.

| Component | Pinned version | Licence / evidence | Use |
| --- | --- | --- | --- |
| Go toolchain / standard library | 1.26.5 | BSD-style [upstream licence](https://go.dev/LICENSE) | Build, HTTP, templates, signals, tests |
| github.com/go-chi/chi/v5 | v5.3.2 | MIT; retained in third_party/chi.LICENSE; [upstream source](https://github.com/go-chi/chi/tree/v5.3.2) | HTTP router |
| github.com/jackc/pgx/v5 | v5.11.0 | MIT; exact notice/index in third_party; [upstream source](https://github.com/jackc/pgx/tree/v5.11.0) | PostgreSQL driver and pool |
| github.com/jackc/tern/v2 | v2.4.3 | MIT; exact notice/index in third_party; [upstream source](https://github.com/jackc/tern/tree/v2.4.3) | Explicit migration command |
| PostgreSQL server | 17.11 in CI; native verification used 17.10 | [PostgreSQL licence](https://www.postgresql.org/about/licence/); server is not bundled in this repository | Development/test database |

go.mod and go.sum pin/verify module dependencies. The [Go notice index](../third_party/README.md) records exact upstream licence texts, versions and hashes for all 19 module dependencies used by the application/worker/migration build. They use MIT, BSD-3-Clause or Apache-2.0 licences; original copyright notices are retained. Dependency copyright attribution is legitimate upstream licence material, not merchant/customer data. sqlc has not yet been introduced.

Frontend: Node 26.5.0; Vue 3.5.43; Vite 8.3.3; plugin-vue 6.0.9; vue-tsc 3.3.12; TypeScript 5.9.3. package.json pins direct versions and package-lock.json locks transitive packages. TypeScript 7 is not currently compatible with this vue-tsc integration. [Frontend inventory and runtime notices](../third_party/frontend-dependencies.md) record versions and publisher-declared licences. Node/Vite are build tooling; no Node server is required. Review all native/tool distribution notices before shipping those tools rather than the browser output.

Gitleaks and pinned GitHub Actions are repository verification tools; they are not application runtime dependencies. Distributable releases must include applicable upstream notices. Naming and dependency checks do not constitute a completed trademark clearance or security audit.
