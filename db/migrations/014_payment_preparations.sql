-- Pre-authorization records only. No submission/paid states or provider tokens.
CREATE TABLE public.payment_preparations (
 id text PRIMARY KEY CHECK(id ~ '^[0-9a-f]{32}$'),
 checkout_id text NOT NULL REFERENCES public.checkout_reviews(id) ON DELETE CASCADE,
 checkout_revision bigint NOT NULL CHECK(checkout_revision>0),
 quote_hash text NOT NULL CHECK(quote_hash ~ '^[0-9a-f]{64}$'),
 idempotency_key text NOT NULL UNIQUE CHECK(idempotency_key ~ '^[0-9a-f]{32}$'),
 customer jsonb NOT NULL CHECK(jsonb_typeof(customer)='object' AND octet_length(customer::text)<=8192),
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object' AND octet_length(snapshot::text)<=131072),
 currency text NOT NULL CHECK(currency='GBP'),
 total_pence bigint NOT NULL CHECK(total_pence>0),
 state text NOT NULL DEFAULT 'prepared' CHECK(state IN ('prepared','cancelled','expired')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX payment_preparation_active ON public.payment_preparations(checkout_id) WHERE state='prepared';

-- Preparations may be discarded with an unpaid checkout, but cannot be rewritten
-- into a different financial intent or reopened after cancellation/expiry.
CREATE FUNCTION public.protect_payment_preparation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.id,NEW.checkout_id,NEW.checkout_revision,NEW.quote_hash,NEW.idempotency_key,NEW.customer,NEW.snapshot,NEW.currency,NEW.total_pence,NEW.created_at,NEW.expires_at)
 IS DISTINCT FROM ROW(OLD.id,OLD.checkout_id,OLD.checkout_revision,OLD.quote_hash,OLD.idempotency_key,OLD.customer,OLD.snapshot,OLD.currency,OLD.total_pence,OLD.created_at,OLD.expires_at)
 OR (NEW.state<>OLD.state AND (OLD.state<>'prepared' OR NEW.state NOT IN ('cancelled','expired'))) THEN
  RAISE EXCEPTION 'payment preparation is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER payment_preparation_immutable BEFORE UPDATE ON public.payment_preparations FOR EACH ROW EXECUTE FUNCTION public.protect_payment_preparation();
