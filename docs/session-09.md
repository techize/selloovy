# Product draft increment 09

Delivered: authenticated create, edit and paginated listing of private products;
plain-text name/description, final GBP pence and certificate owner-name policy.
Schema 6 adds the catalogue without changing existing authentication/shop records.
Owned SQL, transactional create-key hashing and edit revisions guard isolation,
duplicate creates and lost updates. Vue retains uncertain drafts and shows explicit
reload/retry actions without browser storage. No new runtime dependency.

Verification: full Go race/PostgreSQL suite, vet/module/build, sqlc generation,
Vue types/build and synthetic browser QA. Tests include two-shop read/write/list
isolation, origin/JSON/body validation, exact money, idempotent concurrent creation,
changed-key conflict, concurrent edits, pagination, logout and outage. Browser checks
include price errors, saved catalogue after full reload, two-tab conflict/reload and
responsive list. QA uses a disposable database/host, leaving real merchant data intact.

Publication guard, staged/history secret scans and final-head CI are required before
merge. Private planning tracker is maintained separately. Go lesson 09 explains
integer money, idempotency and bounded pagination.

Next: maker variants/stock, product-controlled made-to-order fallback, customer
personalisation, then storefront listing/basket and required section builder.
Photos, publishing, VAT, shipping, payments and full POC acceptance remain open.
