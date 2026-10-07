# Foundation and payment proof

Status: agreed direction; implementation pending.

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
