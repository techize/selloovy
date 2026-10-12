-- Only purpose-keyed fingerprints are retained, never raw email or IP addresses.
CREATE TABLE public.payment_attempt_limits (
 fingerprint bytea PRIMARY KEY CHECK(octet_length(fingerprint)=32),
 window_started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 attempts integer NOT NULL CHECK(attempts BETWEEN 1 AND 100)
);
CREATE INDEX payment_attempt_limits_expiry ON public.payment_attempt_limits(window_started_at);
