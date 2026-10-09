# Maker variants and stock

Products → Variants & stock now stores 1–50 variants per private product draft.
Each variant has a unique name (case-insensitive), optional size and colour-pair
labels, an optional final GBP price override, supply mode and whole stock count.
A blank price inherits the product price; overrides remain fixed when it changes.
Counts are per variant, so a colour pair never borrows another pair’s stock.

The product controls made-to-order fallback, off by default:

| Supply | Stock | Fallback | Saved availability | Dispatch estimate |
| --- | --- | --- | --- | --- |
| Stocked | Positive | Either | In stock | Within 2 working days |
| Stocked | Zero | Off | Unavailable | None |
| Stocked | Zero | On | Made to order | 5–7 working days |
| Always made to order | Zero | Either | Made to order | 5–7 working days |

Always-made-to-order variants require zero stock. Change existing physical counts
explicitly before selecting that mode; the UI never silently discards stock.
Shipping then takes 3–4 working days. These are launch defaults shown in admin,
not calendar-date promises or configured shipping services. Mixed baskets must
ship together when the basket/fulfilment increment is delivered. Full upfront
payment remains the agreed future order policy; no payment occurs here.

## Persistence and access

Schema 7 adds variants, a product fallback flag and stock adjustments while
preserving existing product/shop/authentication data. No real products or variants
are seeded. Names allow 120 Unicode characters, size 80 and colour pair 120. Price
overrides allow £0.01–£1,000,000.00; counts allow 0–1,000,000. Values are plain text.
There is no image upload, stock hold, sales deduction, product publication or
manufacturing/material/cost accounting yet.

GET/PUT /api/admin/products/{id}/maker uses the owner-session/origin/JSON boundary.
PUT is a complete snapshot with the product revision and every existing variant.
New variants have id 0; saved variant IDs must belong to this exact product. Missing
saved variants are rejected, preventing an accidental deletion of stock. There is
no saved-variant deletion route. Bodies are bounded to 96 KiB; unknown fields and
unsafe values fail. Responses are uncached and driver errors are redacted.

Reads use a repeatable-read transaction to keep parent revision and child rows
consistent. Saves revalidate/lock the session and owner, compare the shared product
revision, then write fallback, variants and count adjustments in one transaction.
Product-detail edits and maker saves share that revision. Stale saves conflict;
an uncertain response requires a reload, preventing duplicate new variants or
blindly overwriting counts. Label uniqueness is checked in Go and deferred in SQL
so valid label swaps do not fail midway through the transaction.

Changed counts record the acting owner ID, previous/new quantities, initial-stock
or admin-count reason and database time. Zero-to-zero saves add no adjustment.
The adjustment commits with its stock change; a failed audit insert rolls back the
whole save. A merchant-facing history viewer and operational retention policy are
later work; this is not a completed inventory ledger for order fulfilment.

## Verification and next work

Go/PostgreSQL tests cover isolation, foreign variant IDs, omitted/duplicate variants,
case-insensitive labels, exact prices and inherited overrides, stock/fallback modes,
label swaps, shared revisions, concurrent edits, stock adjustment evidence, induced
rollback, origin/body/selector denial, logout and outage. Browser QA checks independent
colour counts, fallback on/off, override pricing, field validation, full-page reload,
two-tab conflict/reload and narrow-screen overflow. Public scans and final-head CI
must pass before merge.

Next: customer personalisation collection, product photos and controlled storefront
publication/listing and basket; shipping/calendar estimates and the required section
builder remain POC work. Checkout inventory holds and durable order deductions must
be integrated before selling. Square strict 3DS enforcement remains a separate gate.
