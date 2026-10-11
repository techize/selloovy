# Increment 19: Square investigation and read-only readiness

Reviewed current official Square documentation and recorded the UK Risk Manager,
SDK authentication and sandbox evidence gaps. Strict all-card 3DS compatibility
remains unverified. Prepared provider questions have not been sent.

Added a separate CLI to check a selected active UK/GBP sandbox card-processing
location. It uses a fixed GET, private token file, bounded reads/timeouts, redirect
rejection and sanitized output. It cannot create payments or enable checkout.
Synthetic HTTP tests cover account selection, unsafe credentials, redirects,
API failures and response disclosure. No real account request or SDK payment test
has been performed. Credentials and account data remain outside Git.

Documented the proposed durable order-intent/payment-attempt boundary: stable
idempotency, persist-before-submit, unknown-outcome reconciliation, single stock
consumption and transactional notifications. This is design, not implementation.
See square-payment-proof.md, payment-attempt-boundary.md and go-learning-19.md.

Next: configure a UK sandbox, obtain enforceable strict-policy evidence and build
durable payment storage/reconciliation. A successful location check does not
complete the authentication matrix or justify live card payments.
