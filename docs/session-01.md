# Foundation increment 01

Implemented: Go application and worker entry points; validated HTTP configuration; Chi routing; embedded public preview; liveness/readiness separation; graceful HTTP shutdown; focused configuration, route and lifecycle tests. The worker is a lifecycle scaffold, not an operational job processor.

Milestone state: M01-02 implemented; M01-01 partial (Go/Chi pinned; database/frontend setup pending); M01-04 partial (public page/health, no Vue or database); M01-05 research plan present, provider evidence unverified. Full foundation and commerce POC are incomplete.

Next task: wire PostgreSQL with migrations and a real readiness check using an isolated development runtime, then add the Vue/TypeScript admin shell. Keep Square sandbox evidence work separate from ordinary application routes.

Open dependencies: local database runtime, sandbox access and CI publication/verification. No database, provider account, credential, payment or infrastructure change is part of this increment.

Verified locally: module integrity, vet, race-enabled tests and application/worker builds passed. Binary smoke checks confirmed homepage 200, liveness 200, readiness 503 and clean SIGTERM exits for application and worker. GitHub still reports no workflow scope for the active CLI credential; prepared CI cannot be published with that credential yet.

Go explanation and optional exercise: go-learning-01.md. Startup and checks: development.md.
