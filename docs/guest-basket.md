# Guest basket and certificate names

Schema 10 adds private PostgreSQL-backed guest baskets. With a configured public
origin, published products offer Add to basket. The basket supports quantity and
certificate-name updates, removal and continued shopping without JavaScript.
Editable shipping and preview delivery estimates are available; checkout, payments, stock holds and abandoned-basket admin remain open.

## Customer choices and prices

Only reviewed products/options can be added; unavailable options are blocked.
Certificate names follow the published product's none/optional/required policy.
Names preserve spelling/case/internal spacing, trimming outer whitespace only;
invalid Unicode, controls and names above 80 Unicode characters are rejected.
One name applies to the entire line. Add separate lines for different owners.
Matching product, option and name additions merge quantities. Limits are 40 lines
and 1–99 items per line. These are POC bounds, not reservations. Combined demand across personalised
lines is checked against stock: insufficient stock blocks the basket when fallback
is off, or uses product preparation time when fallback is on. Final rechecks and
holds precede checkout later.

All unit prices and subtotals come from the published catalogue in integer pence.
Draft edits do not alter a basket's prices. Republishing does: a reopened basket
shows current values and flags changed prices/details until the line is updated.
Unpublished/unavailable products or changed certificate requirements remain visible
as blocked lines that can be removed. Their reference prices stay in the subtotal,
which is explicitly labelled incomplete. No payment amount or order is created.

## Browser ownership and writes

A random 256-bit bearer cookie identifies a basket within one published shop.
Only its SHA-256 digest is stored. Cookies are host-only, HttpOnly, SameSite=Lax,
scoped to the shop path and Secure with a __Secure- prefix on HTTPS. Explicit
loopback HTTP is for development. Basket/certificate data and tokens never enter
URLs or public catalogue projections; templates escape customer text.

POST requires the configured Host and exact Origin, same-origin fetch metadata
when supplied, a cookie-bound form token, bounded URL-encoded fields and no
unknown/duplicate fields. A CSRF token is derived from the secret cookie; the raw
bearer cookie is never rendered. Referrer-Policy is same-origin so native form
posts retain an origin while external destinations receive no referrer. No-script
CSP and same-origin form-action remain. Browser QA caught and verified this policy
interaction without relaxing origin checks.

Each form carries a basket revision. A row lock, repeatable-read transaction and
revision check prevent stale/double/racing writes; uncertain writes require reload.
Catalogue resolution shares that transaction. Failed writes roll back, and POST
success redirects to GET to avoid routine refresh resubmission.

## Expiry and later work

New basket rows and cookies use 30-day logical recovery. Successful basket changes
and checkout saves refresh expiry; reads do not. Existing rows/cookies retain the
previous expiry until a successful write. Mutation performs bounded opportunistic
removal, excluding unresolved payment reviews. Scheduled physical deletion and
backup retention remain readiness gates; customer details are personal data.

Checkout review and server-only stock-hold primitives are now available. Admin
abandonment visibility, recovery notifications/consent, cross-device resumption,
rate controls, VAT configuration, durable orders and Square strict 3DS proof remain
open. See [checkout.md](checkout.md); this is not a completed test order.

See [shipping.md](shipping.md) for service choices, recalculation and calendar maintenance.
