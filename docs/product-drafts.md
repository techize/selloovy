# Private product drafts

An authenticated owner can create, list and edit their shop’s product drafts in
Products. Names allow 160 Unicode characters, descriptions 5,000 plain-text
characters, and final GBP prices £0.01–£1,000,000.00. Money is stored as integer
pence; no VAT calculation is implied. Certificate owner-name policy is none,
optional or required. This records a requirement, not customer personal data.

Products start private. Reviewed [publishing and public browsing](storefront-publication.md)
are now available separately. [Cover photos](product-photos.md) are available. Basket and checkout remain open. [Maker variants](maker-variants.md), stock counts,
product-controlled fallback and working-day duration previews are now available
in admin. Customer personalisation collection follows.
VAT registration and inclusive/exclusive presentation remain future settings.
There is no delete route, import or seeded real merchant catalogue.

Schema 6 adds products with shop ownership, revisions and a unique shop-scoped
creation key plus normalized request hash. Explicit migration preserves existing
shop/authentication data. The session digest selects the owner/shop in all queries;
client shop IDs are rejected. Writes lock and revalidate the owner/session, then
compare revisions or resolve a repeated creation key in the same transaction.
Concurrent edits from one revision have one winner. An identical repeated create
returns the same product; reusing its key with different content returns conflict.
The returned product reflects current saved values, including later edits.

Lists use 50 items and an ascending ID cursor. GET/POST /api/admin/products/ and
GET/PUT /api/admin/products/{id} share the existing origin/JSON/session boundary.
Responses are uncached; malformed/unknown fields, oversized bodies, invalid values
and foreign product IDs fail without driver errors or merchant identifiers.

An uncertain create retains its original payload/key in component memory, freezes
editing and offers a safe repeat. It is not automatically retried. An uncertain or
conflicting edit keeps the draft and requires authoritative reload. Closing the
workspace discards that in-memory retry state; check the saved list before creating
again. No product drafts or credentials are placed in browser storage.

Verification covers shop isolation for list/read/write, persistence, exact pence,
Unicode/control limits, origin/body/selector denial, repeated and racing creates,
changed-key conflict, concurrent revision edits, pagination, logout and outage.
Synthetic browser QA covers create/edit/reload, inline price errors, two-tab conflict
and reload updating both editor/list, plus narrow-screen overflow. The entire Go
race suite and Vue type/build checks must pass before publication.

Ordinary saves retain the existing public content until republish.
