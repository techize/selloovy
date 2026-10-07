# Dependency record

Reviewed 7 October 2026. Review new dependencies and transitive rights as each feature is added; this record covers only the current Go foundation.

| Component | Pinned version | Licence / evidence | Use |
| --- | --- | --- | --- |
| Go toolchain / standard library | 1.26.5 | BSD-style [upstream licence](https://go.dev/LICENSE) | Build, HTTP, templates, signals, tests |
| github.com/go-chi/chi/v5 | v5.3.2 | MIT; retained in third_party/chi.LICENSE; [upstream source](https://github.com/go-chi/chi/tree/v5.3.2) | HTTP router |

go.mod and go.sum pin/verify the application module dependency. Chi currently has no additional runtime module dependencies in this application. PostgreSQL/pgx/sqlc and Vue dependencies have not yet been introduced or reviewed here.

Gitleaks and pinned GitHub Actions are repository verification tools; they are not application runtime dependencies. Distributable releases must include applicable upstream notices. Naming and dependency checks do not constitute a completed trademark clearance or security audit.
