# Increment 18: checkout review and allocation foundation

Delivered UK guest customer details, saved checkout review, server-derived totals,
current-price/shipping/availability conflict detection, private same-browser
recovery and clear save/validation feedback. The basket links to checkout after
availability and delivery selection. Schema 13 adds bounded review snapshots and
stock holds; successful unpaid writes refresh 30-day logical retention.

Server-only primitives provide 15-minute physical allocations at future payment
start, sorted locks, personalised-line aggregation, partial-stock fallback and
retained uncertain-payment allocations. Owner stock edits cannot invalidate
allocated inventory. Admin displays allocation separately from stock on hand.
No payment route invokes these primitives yet; no order/payment is created.

Go race/PostgreSQL tests verify competing last-item allocation, own-cookie scope,
stale/tampered quotes, expired and retained allocations, pending edit guards,
partial-stock fallback, failed-save rollback, pending financial snapshot stability,
made-to-order nonallocation and unresolved-record cleanup protection. HTTP checks
cover origin, CSRF, duplicate/client-total rejection, escaping, saved feedback and
30-day cookies. Vue types/build, Go vet/build/modules and generated-query checks
are required before publication. Disposable-browser QA verifies product→basket→
shipping→checkout, saving, reload persistence and inline server validation. Only
synthetic data is used; screenshots stay private. Mobile/touch proof is outstanding.

Next: Square strict 3DS proof and durable provider attempts/orders/reconciliation,
then notifications and admin abandonment visibility. No paid checkout or live
readiness is claimed. See checkout.md and the roadmap.
