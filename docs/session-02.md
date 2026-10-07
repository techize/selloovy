# Foundation increment 02

Implemented: bounded pgx connection pool; explicit Tern migration command with embedded forward-only SQL; initial shops table; readiness gated on database connectivity and exact schema version; redacted configuration/database errors; isolated native PostgreSQL helper with ignored credentials and retained data.

Verified locally: vet, race-enabled tests, builds and repeated migrations. Real database integration tests cover reconnect persistence, future-schema refusal and outage behaviour. Process smoke checks stop/restart the dedicated PostgreSQL cluster: liveness stays 200, readiness changes 200/503/200 and a synthetic saved row survives the database restart. Temporary fixtures are removed. Local tests used installed PostgreSQL 17.10; CI uses pinned 17.11.

Milestone state: M01-03 implemented and locally verified; M01-01 still partial until frontend toolchain/setup is pinned; M01-04 has real database readiness but no Vue admin shell yet. Current shop records are database scaffolding, not a merchant setup interface. Worker jobs, MFA, catalogue, payments and POC acceptance remain open.

Next: Vue/TypeScript admin shell and its build/type checks. Square sandbox capability proof remains a separate open investigation. See development.md for startup and go-learning-02.md for the supporting lesson.
