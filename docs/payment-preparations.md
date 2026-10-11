# Pre-authorization payment preparations

Schema 14 adds server-only payment preparations. They are immutable copies of the
saved checkout customer, lines, certificate names, shipping and GBP total, with
server-generated preparation ID and stable idempotency key. A preparation is not
a paid order, a provider submission or evidence of successful 3DS.

PreparePayment locks the owned basket/review, shipping, products and variants in
the existing order. It rejects unsaved/stale reviews and pending financial state.
The snapshot and physical allocation commit together. Concurrent duplicate starts
reuse the ID, key and original deadline; they do not extend the 15-minute window.
Stock remains on hand until a future trusted paid-state transaction consumes it.
Made-to-order units do not allocate physical stock. Product-controlled fallback
uses the existing aggregate allocation logic across personalised lines.

Cancellation is scoped to the owned checkout and exact preparation ID. Repeating
an old cancellation cannot release a newer allocation. Expired or edited
preparations are closed on the next valid start; the new intent receives a new
key. Database constraints/trigger prevent rewriting intent fields, reopening a
terminal preparation or inventing submitting/paid states. Expiry is checked against
database time; there is no scheduled state cleanup job yet.

There are no customer/admin HTTP routes for these methods. No provider token,
card detail or credential is stored. Pre-authorization copies cascade with unpaid
checkout deletion under its existing 30-day recovery retention. Before any
financial submission, implement a separate durable attempt/order lifecycle whose
unresolved/paid evidence cannot be deleted by unpaid basket cleanup. Do not call
a provider with this preparation API or infer financial failure from cancellation.
The old MarkCheckoutPaymentPending method is a low-level allocation-protection
primitive, not authentication or a durable submission boundary.

Database race tests cover simultaneous starts, competing last-item allocation,
store reconstruction/retry, intent immutability, cancellation ownership, stale
shipping/revisions, edited-customer snapshots, expiry/new keys, insert rollback
and pending guards. These use disposable databases and synthetic records. They
do not prove Square enforcement. See payment-attempt-boundary.md for the remaining
submission/reconciliation/stock-consumption/outbox design.
