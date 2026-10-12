# Square sandbox adapter

internal/square is a fixed-sandbox HTTP boundary, connected to explicitly configured sandbox checkout; see square-checkout.md.
NewSandbox loads an owner-only regular token file and binds a selected location.
There is no live endpoint/configuration switch. Redirects are refused; requests
have context/client deadlines, bounded responses and sanitized outcomes.

Charge accepts a fresh card nonce, a server-owned stable key/reference and GBP
amount. Source types CASH, EXTERNAL and stored-card IDs are rejected. It sends
customer_initiated=true, seller_keyed_in=false and autocomplete=true. Billing
verification happens through the browser SDK before this eventual call. Requests
must be durably persisted before submitting; there is no automatic network retry.

Paid requires a matching location, reference, currency/amount and CARD source,
with payment COMPLETED, card CAPTURED and a valid capture timestamp. APPROVED/PENDING remains pending;
matching FAILED/FAILED or CANCELED/VOIDED is a definitive decline. Explicit
allowlisted card-method errors in a POST 400/402 without a payment record also
decline the current request; idempotency/configuration conflicts and used/expired-token ambiguity remain unknown. Invalid,
inconsistent, unauthorized, server-error, timed-out and malformed responses remain
unknown. Reconcile is a GET for a known ID and validates the same immutable intent.
Unknown outcomes must not release stock or trigger a fresh-key charge.

Implicit formatting/JSON of request/client/result objects omits private data.
This is not comprehensive secrecy: callers must not log individual source-token,
credential, customer or provider-ID fields. Provider errors/bodies are not returned.
Only the future durable order/attempt layer may decide stock/order transitions.

Transport tests cover matching capture, declines, pending, response mismatches,
redirect/credential boundaries, cancelled/failed transport, size bounds and
private formatting. Explicit opt-in real UK/GBP sandbox tests use official synthetic
API source fixtures: one captured simulation and one declined simulation; repeated
same-key calls return the same provider ID/outcome and GET reconciliation matches.
No real funds/cards or Selloovy checkout order is created by these adapter tests.

Use SELLOOVY_RUN_SQUARE_SANDBOX_TESTS=yes, private token-file/location environment
configuration and go test -race ./internal/square -run TestActualSandboxAdapter
only when deliberately exercising the selected sandbox. Default tests/CI do not
contact Square. Never put credentials in shell arguments or public fixtures.
Current SCA policy is payment-policy.md; the original universal-3DS gate is retired.
