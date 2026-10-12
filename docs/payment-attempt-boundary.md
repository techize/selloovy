# Payment attempt boundary — proposed next implementation

Status: financial lifecycle remains design, not delivered. Checkout reviews,
server-only allocations and immutable pre-authorization preparations exist (see
payment-preparations.md); no durable financial submission or paid order exists yet. Square standard SCA under payment-policy.md is now approved; complete durable
financial lifecycle and abuse controls before enabling an adapter. This design lets storage and
reconciliation work proceed while that evidence is pending.

1. **Prepare:** lock/recheck the owned saved checkout, resolve current total and
   availability, and create an immutable order-intent snapshot plus a server-owned
   attempt ID and stable provider idempotency key. Enforce one unresolved attempt
   per checkout. Preserve exact line prices, certificate owner text, shipping
   service/charge, GBP total and customer details. A checkout review itself remains
   editable before this boundary; the financial snapshot must not be overwritten.
2. **Authenticate:** acquire the physical allocation when payment begins. Give the
   provider browser component authoritative amount/currency and appropriate billing
   details. Collect billing details explicitly, including whether they match
   shipping; do not assume they do or split a full name heuristically. No card
   number/CVV should pass through Selloovy. Failed/cancelled tokenization must stop; Square determines required SCA and
   declines must not be overridden. A browser success flag is not payment proof.
3. **Persist before submission:** after verified policy and fresh review/hold
   checks, atomically mark the attempt as submitting and retain its allocations.
   Only then make the provider request. A crash before/after that call leaves a
   durable unresolved attempt, not an editable basket capable of another charge.
   The earlier MarkCheckoutPaymentPending primitive alone is insufficient.
4. **Resolve:** trust verified provider observations matching the attempt, account,
   amount/currency and reference. Record paid state and consume allocated physical
   stock exactly once in the same transaction; made-to-order quantities create
   production demand rather than negative physical stock. A definitive failure
   permits release once. An uncertain outcome retains allocation and requires
   reconciliation with the same attempt/key before retry or new payment.
5. **Notify and recover:** persist notification jobs in the successful state-change
   transaction. Retry delivery separately. Expose a sanitized owner order/payment
   view and escalate unknown outcomes after five minutes. Customer-visible status
   must distinguish pending, failed and paid. No fulfilment begins on a browser
   redirect or a merely authorized, uncaptured payment.

Proposed state paths: prepared → authenticating → submitting → paid / failed /
unknown. Unknown can resolve to paid or failed only through trusted provider
reconciliation. Authentication cancellation can end a prepared attempt without
submission. State names, cancellation behavior and the provider query/retry contract
must be finalized in implementation. Transport failure is not a definitive decline.

Before wiring a provider, tests must cover duplicate clicks, concurrent starts,
replayed browser tokens, amount changes, mismatched provider/account observations,
webhook authentication and duplicates/out-of-order arrival, crash before/after
submission, unknown outcomes, retry with the same key, single stock consumption,
partial-stock fallback, notification retry and transactional rollback. Fake-adapter
passes prove application behavior only; they are not a Square payment/3DS pass.

Never store raw card details. If retry needs a source token/request envelope,
define minimum encrypted storage, key management and retention before implementation;
it must not enter ordinary logs, snapshots, exports or public fixtures. Provider
credentials belong outside the public repository. Read-only sandbox configuration
checks live separately from merchant checkout; see square-payment-proof.md.
