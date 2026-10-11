# Increment 20: immutable payment preparations

Delivered server-only immutable pre-authorization intent storage and stable retry
keys, atomically saved with 15-minute stock allocations. Duplicate starts reuse
the intent/deadline, old cancellation cannot release newer holds, and expired or
edited intents create new keys. Snapshot fields and terminal transitions are
protected in the database. Made-to-order units do not allocate physical stock.

Schema 14 and sqlc queries are added. Synthetic disposable-database race tests
verify concurrency, ownership, retries, stale/edited intent, immutability, expiry,
rollback and pending-state guards. There is no new browser workflow or provider
submission. Existing checkout review remains available; strict 3DS proof is open.

Next: durable financial attempts/orders with persist-before-submit, verified
reconciliation, exactly-once stock consumption and notifications. Before financial
submission, replace unpaid-cascade retention with a durable financial lifecycle.
See payment-preparations.md, payment-attempt-boundary.md and go-learning-20.md.
