# Current payment policy

Approved 12 October 2026: Square remains the processor; accept its standard
SCA/3DS and risk decisions. This explicitly supersedes the initial requirement
for successful 3DS on every card and for a separate universal pre-authorization
proof. Provider exemptions or transactions without required 3DS may be accepted
when Square accepts them. Do not advertise that every card is 3DS-authenticated.

Always use the current Web Payments SDK Card.tokenize(verificationDetails) flow
with server-derived amount/currency, explicit billing contact, CHARGE intent,
customerInitiated=true and sellerKeyedIn=false. No merchant-facing bypass option,
MOTO flag, stored-card shortcut or bare tokenize() path. Reject failed/cancelled
browser tokenization, and never override an API verification-required decline.
A browser token/OK flag is not proof of payment. Only a matching trusted Square
COMPLETED/captured result may make an order paid; APPROVED alone is insufficient.

Card numbers/CVV stay inside Square's browser components. Source tokens and
provider credentials never enter logs, public Git or ordinary merchant exports.
Server-owned attempts/amounts/idempotency, persisted before submission, remain
mandatory. Unknown outcomes retain allocations and reconcile before another
charge. Stable keys are reused for retries; no fresh key after a timeout.

Prevent card-checking with bounded checkout/payment requests, persistent shared
attempt throttles and limits, origin/CSRF protection, basket/customer validation
and one unresolved payment per checkout. Billing/amount changes require fresh
review/tokenization. Generic customer errors must not expose raw provider details.
These controls reduce abuse; they do not guarantee elimination of stolen-card use.

Square sandbox comes first; no automatic switch to live credentials or wallets.
Wallets, stored cards, refunds and production activation remain separate scoped
work. Native SCA is accepted, but durable orders/reconciliation, stock consumption,
notifications and end-to-end acceptance still gate enabling checkout.

Basis: [Square card flow](https://developer.squareup.com/docs/web-payments/take-card-payment)
and [SCA overview](https://developer.squareup.com/docs/sca-overview).
Measured sandbox observations and historical strict-policy gaps are retained in
square-payment-proof.md. No additional provider subscription is assumed.
