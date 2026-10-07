# Foundation and payment proof

Status: foundation implemented and verified; Square investigation remains open.

Go/Vue previews, PostgreSQL migration/readiness, startup/shutdown and CI are
verified through the first three implementation increments. The capability
matrix is prepared; account-dependent Square enforcement evidence remains open.
Owner credential helpers now support the next milestone, but no merchant login
or commerce flow is implemented. Historical task descriptions below specify
acceptance; they do not imply that provider evidence is complete.

## Build backlog

Tasks describe deliverables and evidence, not promised durations. Complete foundation tasks while account-dependent payment evidence is pending. Track each task as pending, in progress, verified or blocked; an untested implementation is not verified.

| Task | Deliverable and acceptance evidence | Dependency | Go learning |
| --- | --- | --- | --- |
| M01-01 | Record supported Go, Node and PostgreSQL versions; choose/pin dependency versions and licences. Document local startup, ignored configuration and clean teardown that preserves data unless explicitly requested. A fresh checkout can follow the instructions. | Local database runtime available | Modules, packages and reproducible builds |
| M01-02 | Add Go application and worker entry points, validated configuration, signal handling and graceful shutdown. Missing required configuration fails clearly without printing its value. | M01-01 | Functions, errors, structs and context |
| M01-03 | Add PostgreSQL connection pooling and a first versioned migration. Migration runs successfully on an empty disposable database and reruns safely; data survives a restart. Test fixtures are synthetic. | M01-02, database | pgx, contexts, SQL and migration ownership |
| M01-04 | Serve a minimal Go-rendered public page and built Vue/TypeScript admin shell from the application. Add liveness and readiness endpoints: liveness does not depend on PostgreSQL; readiness fails if PostgreSQL is unavailable. Neither endpoint reveals configuration. Admin has no privileged functionality before access controls exist. | M01-02/03 | HTTP handlers, routing and templates |
| M01-05 | Define the payment capability matrix and proposed adapter boundary. Trace amount/currency, authentication and authorization separately. Record which trusted provider signal or account control can enforce each condition; do not accept a browser-provided success flag as evidence. | Source research; independent of full storefront | Interfaces and explicit outcome types |
| M01-06 | Run a restricted Square sandbox investigation using ignored credentials and provider test inputs. Cover each scenario in square-payment-proof.md; publish sanitized conclusions only. A provider limitation is a valid investigation result, not a passed payment gate. | M01-05, sandbox access | HTTP clients, error handling and idempotency |
| M01-07 | Publish CI once GitHub authentication permits workflow changes. Verify public guard, secret scan, Go build/tests and frontend type/build checks; add applicable checks as implementation exists. Require verified checks on main. | Workflow permission; M01-02/04 for app checks | Testable boundaries and table-driven tests |
| M01-08 | Demonstrate startup, storefront/admin shells, database migration/readiness and shutdown. Record commands, evidence, outstanding payment cases and the next task. Leave a short Go explanation and optional exercise. | Foundation tasks; payment investigation status disclosed | Reading the request path end to end |

Start with M01-01/02 and M01-05. A provider probe stays separate from merchant checkout until payment policy and access controls are established. Do not expose the probe publicly or add unauthenticated payment routes to the ordinary application.

## Milestone completion

Foundation acceptance below must pass. Payment investigation must produce a capability decision: proven for the tested sandbox conditions, unsupported, or still unverified with specific missing evidence. Missing credentials or untested failure cases mean the investigation remains open. Live checkout stays gated until live-account enforcement is verified separately. CI permission is an explicit outstanding task, not a waived check.

The visible demo is a foundation demonstration. It is not the later maker setup-to-test-order POC, which also requires authentication, catalogue, page builder, stock, shipping, orders and notifications.

## Foundation acceptance

- A fresh checkout has documented commands to start a local Go application, PostgreSQL and Vue admin build.
- The public storefront is rendered by Go; admin calls the authoritative Go API.
- Database migrations and a health check demonstrate connectivity without exposing credentials.
- Public configuration is generic, dependency versions/rights are recorded, and secrets come from local ignored configuration.
- Build, formatting/type checks and meaningful verification run locally and in CI.
- A short session note records what works, blockers, next action and the Go concept learned, without personal or operational details.

## Payment investigation acceptance

Document the Square flow and evidence for successful frictionless/challenge authentication, failed/cancelled authentication, unsupported/unavailable authentication, missing proof, retries and browser interruption. Distinguish token creation from authorization and server-side enforcement. The requirement is to block unsuccessful or unavailable authentication before an unauthenticated authorization, rather than refunding afterwards.

Use sandbox credentials kept outside Git and synthetic provider test data. Record sanitized outcomes and unresolved cases. Sandbox success alone does not prove live-account capability. Wallets and exemptions need a separate policy; no unapproved exception or automatic provider fallback.

If Square cannot demonstrably support the policy, report the limitation and options for a maintainer decision. Continue independent foundation work, while keeping live checkout gated. Do not claim that this investigation is complete merely because a request asks for 3DS.
