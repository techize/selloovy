# Delivery checkpoint: measured Square sandbox evidence

Verified selected UK/GBP account access and isolated browser probes using observed
SDK 1.86.0 and API 2026-09-16. Challenge/frictionless fixture paths return tokens;
cancelling a challenge produces no replacement token. Failed-verification fixture
returns browser OK but the API rejects it with CARD_DECLINED_VERIFICATION_REQUIRED.
An ordinary fixture tokenized without verification details is accepted as a
simulated authorization; it was cancelled and cancellation verified by provider GET.
Both records include an authorization timestamp without capture; the failed record
does not prove a pre-authorization rejection boundary.

No real cards/funds, live API, real customer data or main checkout were used.
Temporary browser/server/source tokens removed; private sanitized metadata retained.
No runtime, schema, policy relaxation, support message or provider switch.
Strict all-card enforcement remains unresolved and needs a maintainer decision.
See square-payment-proof.md for measured results and limitations.
