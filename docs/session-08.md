# Shop settings increment 08

Implemented: authenticated shop name, tagline, plain-text description and optional
customer contact email editing. UK launch defaults display read-only. A separate
shop module, HTTP adapter and sqlc query package enforce session-owned access and
transactional revision checks. Schema 5 retains existing shop names.

Verification: local Go/PostgreSQL race tests, vet/modules/build, query generation,
Vue types/build and synthetic browser QA. Tests cover cross-shop isolation, rejected
client selectors, origin/body limits, validation, saved data, stale revisions, one
winner in simultaneous edits, logged-out writes and dependency outage. Browser checks
cover save, reload, field errors, a two-tab conflict and mobile overflow.

Publication scans and remote CI must pass before merge. No real merchant settings
were read or changed during QA; disposable database/credentials removed. Saved identity
is admin-only; no storefront publication, VAT/shipping change, email or payment occurs.

Next: catalogue product creation and listing before maker variant/stock workflows and
section-builder publication. Square strict 3DS evidence and full POC acceptance remain
open. Go lesson 08 explains module boundaries, owned SQL and optimistic concurrency.
