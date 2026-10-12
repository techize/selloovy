# Persistent payment attempt limits

Every new financial attempt consumes three database budgets in the same transaction
as its financial intent. All three must succeed before a provider call is possible:

- Five attempts per checkout per 15-minute window.
- Ten attempts per shop/customer email per 15-minute window.
- Twenty attempts per connection address/network per 15-minute window.

A definitive decline consumes the budget too. Duplicate requests for an existing
attempt return its saved state without consuming another budget or contacting Square.
Rejected, invalid or rolled-back intents never call the provider. Counters survive
application restarts. These are fixed windows starting at the first attempt, so
traffic can be higher around a window boundary. They are initial conservative
limits, not a complete fraud-detection system.

Fingerprints use a separate HKDF-derived HMAC-SHA256 purpose key from the installation
key. Raw email and connection addresses are not stored in the budget table. Email
normalization uses trim/lowercase; IPv4-mapped addresses normalize to IPv4 and IPv6
addresses share a /64 network budget. Sorted fingerprint updates avoid inconsistent
lock ordering. Expired fingerprint cleanup removes at most 1,000 records older than
one day, leaving current budgets and financial evidence intact.

The caller must supply the validated socket address. Forwarded headers are not
trusted. A future reverse-proxy deployment needs an explicit trusted-proxy policy;
unconfigured proxies will conservatively share one connection budget. Do not disable
budgets to fix that symptom. Installation-key rotation resets fingerprints and needs
the same controlled operating procedure as payment-envelope and MFA key rotation.

Verification covers persistent checkout limits, free duplicate reads, transactional
rollback, window reset, cleanup, email limits across separate baskets, and rotating
IPv6 addresses within one /64. Explicit sandbox configuration enables the tested browser checkout; live processing
is unavailable. See square-checkout.md.
