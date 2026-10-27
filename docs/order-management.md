# Merchant orders and fulfilment

The authenticated Orders workspace lists recent payment attempts with saved customer
name/email, amount, date, payment outcome and preparation/dispatch state. Pagination
uses stable creation-time/ID ordering. An all-shop attention count and dedicated
filter expose unresolved attempts older than five minutes, even beyond the newest
page of orders. The latest 50 saved unfinished checkouts appear separately; more
than one hour since the last saved checkout change is labelled abandoned. Recovery
emails remain deferred. Expired unpaid data is excluded under the recovery policy.

Order details preserve the financial snapshot, exact certificate name, delivery
service and charge, customer address, physical/production allocation and confirmation
job status. Payment tokens/envelopes, source/basket digests, idempotency keys and
provider credentials never appear in the owner API projection. All list/detail,
pagination, dispatch and reconciliation operations check the current owner session
and shop scope. Anonymous/other-shop access cannot read or mutate an order.

Only a confirmed paid order has a fulfilment record. The paid transaction initializes
it with inventory consumption and confirmation jobs. The merchant prepares the
whole order and marks it dispatched together, optionally recording carrier, tracking
reference and an HTTPS tracking link. Saved feedback is visible. Revision checks
reject stale forms; later tracking edits retain the original dispatch timestamp.
A dispatched order cannot be reopened through this flow. Dispatch updates neither
financial totals nor stock consumption, and does not enqueue another receipt.

Customer receipts show the recorded shipment and safe external tracking link.
After dispatch, transit estimates use the recorded date and the saved shipping
service's working-day range rather than adding preparation time again. Before
dispatch, estimates use the confirmed Square capture time. Certificate names preserve
internal whitespace in admin, basket, checkout and receipt displays. Production
materials/costs, split fulfilment, carrier labels and shipment-email delivery remain
later work.

Unknown/pending/submitting attempts cannot be dispatched. For known Square IDs,
the owner can request a matching GET observation. Without a known ID, the owner
supplies the payment ID found through Square's payment records/API for the matching
reference. The adapter verifies location, reference, GBP amount, CARD source,
completed/captured state and timestamp; an unmatched result cannot mark an order
paid. There is no owner-supplied paid/failed flag or another charge request. If no
matching Square evidence exists, keep the attempt unresolved for investigation.

Leaving an edited shipment or changing orders prompts before discarding unsaved
changes. Confirmation status is shown independently of payment status. Queued means not
sent; sending means a request started; validated means test validation without a
message; sent means provider acceptance, not inbox delivery. Failed and unknown
email outcomes do not change a paid order or ask the customer to pay again.

The orders list flags paid orders with rejected/uncertain confirmation requests,
or requests still sending after two minutes. A separate **Show email issues**
filter finds those orders, including ones older than the first page. The alert
counts affected orders across the shop, counting two affected jobs on one order
once. Payment-attention filtering and counts remain separate. Queued and validated
jobs are not email issues. The API returns status/attention flags only, without
rendered payloads, submission keys or provider message identifiers. Owner/shop
and current-session checks apply to the list, counters and pagination.

The POC worker remains limited to explicit Brevo drop validation. Account access
is required to prove provider validation; actual email delivery stays disabled.
There is no resend or owner-supplied delivery-success action. Review provider
activity before any future resend of an uncertain request.

Verification includes PostgreSQL-backed owner/shop/session isolation, private API
projection, unpaid dispatch denial, database fulfilment guards, revision conflicts,
tracking URL/control-character rejection, immutable financial evidence, retained
first dispatch time, verified operator recovery and attention filtering. Isolated
browser QA with synthetic records covered order/unfinished lists, saved dispatch
feedback and unsaved-change protection. Separate actual Square browser checkout
proof is documented in square-checkout.md. No real merchant data or outgoing
messages were used in those tests.
