# Durable financial lifecycle

The server-side payment service is implemented and tested with synthetic providers.
The sandbox checkout HTTP caller and recovery worker are implemented; see
[square-checkout.md](square-checkout.md). Owner order management and email delivery remain required
before enabling customer payments. Persistent new-attempt budgets are implemented;
see [payment-attempt-limits.md](payment-attempt-limits.md). Only the fixed Square sandbox client exists.

`catalog.Payments.Submit` uses the owned, saved checkout and unexpired preparation.
A transaction rechecks the current quote and physical holds, copies immutable
customer and line/shipping snapshots, binds an encrypted source to that intent,
and marks the preparation submitted and checkout pending. It commits before any
provider request. The amount, reference and idempotency key are server-owned.
Concurrent duplicate submissions return the persisted state without another charge.
A source digest prevents reuse of one nonce for another attempt in that shop and
Square location. There is one unresolved attempt per checkout.

The provider call runs outside database locks. Disconnecting the customer does not
cancel saving an observed result: a separate, bounded context handles persistence.
Paid requires Square's matching completed/captured observation and capture timestamp.
The paid transaction consumes physical quantities, records made-to-order quantities,
advances affected product revisions, completes the checkout, releases holds, erases
the encrypted source, and inserts the two notification jobs together. Duplicate
resolution cannot repeat stock consumption or enqueue more jobs. A completed basket
cannot be edited or submitted again; the upcoming receipt flow will start a fresh
basket for continued shopping.

A definitive decline releases holds and erases the source without consuming stock;
a fresh preparation and fresh nonce may then be used. Unknown/pending outcomes retain
allocation and block edits and another attempt. A trusted provider ID is persisted
separately before the stock/outbox transaction, so a failed local commit can be
recovered through a matching Square GET. Recovery never creates a charge. Without
a provider ID, operator reconciliation is required; no automatic resend or fresh-key
retry is allowed. Unknown payments must not be treated as failed or fulfilled.

Financial snapshots and allocation/notification evidence survive removal of a
terminal checkout and basket. Database triggers reject financial snapshot changes,
provider-ID replacement, terminal-state reopening and detaching unresolved attempts.
The encrypted source expires after one hour. `EraseExpiredSources` clears at most
1,000 expired envelopes per call without changing financial state or releasing
uncertain allocations. The worker invokes it every 15 seconds in bounded batches; run the worker with
the application. A stopped worker cannot enforce timely source cleanup. Ordinary financial
exports must omit source digests/envelopes and idempotency/provider credentials.

Payment time is Square's `card_payment_timeline.captured_at`, not the later local
reconciliation time. Missing or malformed capture timestamps leave a matching
completed observation unknown until valid evidence is available. See the
[Square timeline reference](https://developer.squareup.com/reference/square/objects/CardPaymentTimeline).
Shipping date calculation will use this time and the approved working-day calendar.

Verification uses disposable PostgreSQL databases. It covers persistence before
provider I/O, simultaneous submissions, ownership, definitive declines/fresh-token
retry, nonce replay, uncertain allocations, GET-only recovery, stock/outbox rollback,
terminal idempotency, immutable snapshots, basket cleanup and source expiry. These
are application tests, separate from actual Square sandbox/API proof.
