# Maker variants increment 10

Delivered: private product variants with independent stock, size/colour-pair labels,
optional GBP price overrides, stocked/production supply and product-controlled
made-to-order fallback. Saved availability shows stock dispatch within two working
days or production in 5–7, followed by 3–4 working-day shipping defaults. No live
checkout or calendar-date promise is implied.

Schema 7 preserves earlier records and adds variants/fallback/stock adjustments.
Owned transactional saves validate the complete variant set and shared product
revision. Count changes and actor/previous/new history commit atomically. Snapshot
reads keep revision and children consistent. Vue uses expandable editors, inline
errors and explicit reload after stale or uncertain saves. No runtime dependency.

Verification: full Go race/PostgreSQL suite, vet/build/modules, sqlc regeneration,
Vue types/build and isolated synthetic browser QA. Tests cover isolation, variant
injection/omission, price inheritance, stock modes, uniqueness swaps, stale/racing
writes, count-history rollback, HTTP boundaries, logout and outage. Browser checks
cover saved counts, fallback, override pricing, duplicate-name errors, reload and
two-tab conflicts plus responsive layout. Publication scans and final-head CI are
required before merge. Real merchant records remain untouched by QA.

Next: customer personalisation, photos and storefront publication/listing/basket.
Section builder, shipping/calendar dates, Square policy proof, stock holds/orders,
notifications and full POC acceptance remain open. Go lesson 10 explains nullable
prices, snapshots and atomic stock history.
