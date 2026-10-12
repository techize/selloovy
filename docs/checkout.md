# Checkout review and stock allocation foundation

After applying schema 13 and restarting, a nonempty, available basket with a
selected shipping service offers **Continue to checkout**. Customers can save UK
contact/address details, review product and shipping totals, certificate names,
combined preparation and estimated arrival. Saved details survive restart in the
same browser and shop. Saving takes no payment and does not reserve stock.

The server derives prices, shipping and availability from the published catalogue
and current shipping settings. Each form carries a checkout revision and a hash
of that resolved quote. Changes require another review/save; browser amounts and
unknown or duplicate fields are rejected. Customer validation checks required,
bounded plain text, email syntax, country GB and bounded postcode text. It does
not verify that an address/postcode exists or is deliverable. All UK regions remain
supported. Current prices are final GBP prices without added VAT; future VAT
configuration remains open. A saved review can be replaced until payment starts;
it is not yet an immutable paid order.

## Allocation engine

The server-only HoldCheckout method is intended for payment initiation. No HTTP
route currently invokes it. It requires a saved, unchanged, available review.
A transaction locks basket, review, shop, products and variants, with ascending
product/variant IDs and a consistent lock order. Competing checkouts serialize on
variant rows. Physical availability subtracts active or retained holds; an owned
checkout excludes its own allocation through a cookie-derived database lookup,
never a browser-provided checkout ID.

Physical holds last 15 minutes. Retrying an active hold does not renew its deadline.
Demand aggregates across differently personalised lines. Made-to-order units need
no physical allocation. Where the product permits fallback, only the remaining
physical units are held and the excess uses made-to-order preparation. No stock
count is decremented at this stage. Admin shows stock on hand and allocated stock;
it rejects reductions below allocation or a supply-mode change while allocated.
Saving a new review or editing an unpaid basket releases unsubmitted holds.

MarkCheckoutPaymentPending is another server-only primitive. Before future
provider submission it requires the reviewed quote and an active hold deadline,
then retains allocations and blocks basket/customer edits. Retained allocations
never expire merely because a financial outcome is uncertain. The customer
summary uses the saved financial snapshot during that state. This primitive is
not a payment adapter: durable provider attempts, idempotency, reconciliation,
release/consume transitions and operator escalation must be delivered together
before connecting it to any provider request.

## Privacy and recovery

The existing shop-scoped, HttpOnly bearer cookie and its stored SHA-256 digest
identify checkout. GET responses are no-store; POST requires exact origin, host,
CSRF, bounded forms and single known fields. Customer text is HTML-escaped and
never placed in public catalogue data, URLs or request logs. Protect the database
and backups because they contain personal details. Anyone holding this browser's
basket cookie can access its saved checkout; cross-device recovery is not provided.

New baskets/reviews expire logically after 30 days; successful basket writes and
checkout saves refresh database expiry and cookie lifetime. Existing basket rows
and cookies retain their previous expiry until a successful write. Reads do not
extend retention. Opportunistic basket cleanup is bounded and excludes unresolved
payment reviews; scheduled deletion and backup expiry remain readiness gates.
Unresolved financial records require a separate reconciliation/retention policy.

## Remaining delivery

Payment submission stays disabled. Square remains the processor under the approved standard SCA policy
in payment-policy.md; required-authentication failures/provider declines are blocked. Durable orders, provider attempts/webhooks, paid-stock consumption,
notifications, admin abandonment views, one-hour abandonment classification,
operator alerts, scheduled cleanup and checkout abuse limits remain open. No
recovery emails are sent. This increment is not a completed provider test order.
