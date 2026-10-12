# Square sandbox checkout

The local storefront can now complete a Square sandbox test order. Live processing
is not available. Sandbox configuration is explicit and complete: set
`SELLOOVY_SQUARE_SANDBOX_ACCESS_TOKEN_FILE`,
`SELLOOVY_SQUARE_SANDBOX_LOCATION_ID` and `SELLOOVY_SQUARE_SANDBOX_APP_ID`, alongside
the existing database, installation key and public origin. The application ID must
be a sandbox ID. The SDK requires HTTPS or exact `http://localhost`; an HTTP IP
origin is rejected when configuring Square. With localhost configured, local document
navigation from an IP alias redirects to localhost; API and POST requests retain
their exact-origin guards. Existing owner sessions on the old hostname require a
fresh login. Keep token files outside Git, regular,
private and readable only by the application owner. Never regenerate an existing
installation key to configure payments.

Save checkout details and accept the current total, then choose secure test payment.
Preparing acquires an expiring stock allocation. The browser displays the saved GBP
total and collects explicit billing given/family names, email and either confirmed
matching delivery address or a separate billing address. Square hosts the card
fields; card numbers/CVV never pass through Selloovy. Every tokenization supplies
amount, currency, CHARGE intent, billingContact, customerInitiated=true and
sellerKeyedIn=false. Cancellation and unsuccessful tokenization stop submission.
Square determines the required authentication under its standard policy.

The submission endpoint accepts only the saved checkout revision, CSRF and fresh
source nonce. It checks exact Host/Origin, shop-scoped HttpOnly guest cookies,
fetch-origin metadata, the custom request header, JSON content/size/field limits,
server-owned intent and persistent attempt budgets. Browser amounts, locations,
idempotency keys and provider success flags are not accepted. Connection budgets
use the socket IP, never forwarded headers.

Confirmed receipts preserve products, prices, certificate names, shipping and
customer details from the immutable financial snapshot. Dispatch/arrival estimates
use Square capture time and the working-day calendar. Declined payments return to
checkout for a fresh attempt. Pending/unknown states prohibit another payment and
show a private status view; after five minutes that view directs the customer to
the shop owner. Checkout also links back to its current payment for recovery.

A paid receipt sets a scoped HttpOnly receipt cookie, keeping it accessible for
30 days from the attempt while the normal basket cookie is replaced for continued
shopping. Viewing an older receipt cannot replace an already newer basket. Receipt
access needs the matching shop/cookie, including after terminal basket cleanup;
random order IDs alone grant no access. Cookies and customer details never enter
payment URLs. Do not treat these browser cookies as email receipt links.

Run the worker with the same private configuration using `go run ./cmd/worker`, or
`/app/worker` in the container. Every 15 seconds it performs bounded source/budget
cleanup and claims a leased batch of up to five known-provider payment observations.
Leases prevent simultaneous workers from selecting the same due work; retries
perform matching GETs only. Unknown provider IDs require operator reconciliation.
This worker does not send confirmation emails yet. The paid transaction queues
customer/owner notifications; delivery is next. Owner order management and verified reconciliation are available;
see order-management.md.

The payment page has a dedicated sandbox CSP. Besides Square's SDK/card/PCI and
font endpoints, frame-src/form-action allow the exact `https://api.squareupsandbox.com`
origin used by the SDK's simulator challenge. The vendor's basic CSP example alone
blocked that challenge in browser testing. Other pages keep their existing policy.
Payment HTML uses same-origin referrer policy so ordinary same-origin POST forms
retain usable Origin headers; cross-origin requests receive no referrer. JSON
payment responses use no-referrer. Scripts do not log provider tokens/errors.

Verification: disposable-database HTTP tests cover ownership, Host/Origin/CSRF,
unknown fields/amount/key manipulation, duplicate charges, paid cancellation denial,
receipt-cookie scope and continued shopping. Worker tests cover leased GET-only
reconciliation. Actual isolated browser tests completed challenged and frictionless
Square test-card payments, cancellation stopped submission, and each successful
order left one paid attempt, two queued jobs and one physical stock deduction.
No real cards/funds, production database orders or outgoing emails were used.

References: [Square card flow](https://developer.squareup.com/docs/web-payments/take-card-payment)
and [Square CSP guidance](https://developer.squareup.com/docs/web-payments/content-security-policy).
