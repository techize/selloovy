-- Financial evidence is independent of unpaid basket/review cleanup.
ALTER TABLE public.checkout_reviews ADD COLUMN completed boolean NOT NULL DEFAULT false;
ALTER TABLE public.payment_preparations DROP CONSTRAINT payment_preparations_state_check;
ALTER TABLE public.payment_preparations ADD CHECK(state IN ('prepared','cancelled','expired','submitted'));
CREATE OR REPLACE FUNCTION public.protect_payment_preparation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.id,NEW.checkout_id,NEW.checkout_revision,NEW.quote_hash,NEW.idempotency_key,NEW.customer,NEW.snapshot,NEW.currency,NEW.total_pence,NEW.created_at,NEW.expires_at)
 IS DISTINCT FROM ROW(OLD.id,OLD.checkout_id,OLD.checkout_revision,OLD.quote_hash,OLD.idempotency_key,OLD.customer,OLD.snapshot,OLD.currency,OLD.total_pence,OLD.created_at,OLD.expires_at)
 OR (NEW.state<>OLD.state AND (OLD.state<>'prepared' OR NEW.state NOT IN ('cancelled','expired','submitted'))) THEN
  RAISE EXCEPTION 'payment preparation is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TABLE public.payment_attempts (
 id text PRIMARY KEY CHECK(id ~ '^[0-9a-f]{32}$'),
 shop_id bigint NOT NULL REFERENCES public.shops(id),
 checkout_id text REFERENCES public.checkout_reviews(id) ON DELETE SET NULL,
 basket_digest bytea NOT NULL CHECK(octet_length(basket_digest)=32),
 revision bigint NOT NULL CHECK(revision>0),
 idempotency_key text NOT NULL UNIQUE CHECK(idempotency_key ~ '^[0-9a-f]{32}$'),
 location_id text NOT NULL CHECK(location_id ~ '^[A-Za-z0-9_-]{1,80}$'),
 quote_hash text NOT NULL CHECK(quote_hash ~ '^[0-9a-f]{64}$'),
 customer jsonb NOT NULL CHECK(jsonb_typeof(customer)='object' AND octet_length(customer::text)<=8192),
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object' AND octet_length(snapshot::text)<=131072),
 total_pence bigint NOT NULL CHECK(total_pence BETWEEN 1 AND 1000000000000),
 source_digest bytea NOT NULL CHECK(octet_length(source_digest)=32),
 source_cipher bytea CHECK(octet_length(source_cipher) BETWEEN 28 AND 4124),
 source_expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '1 hour',
 state text NOT NULL DEFAULT 'submitting' CHECK(state IN ('submitting','pending','unknown','paid','failed')),
 provider_id text CHECK(provider_id ~ '^[A-Za-z0-9_-]{1,80}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 paid_at timestamptz,
 UNIQUE(shop_id,location_id,source_digest),
 UNIQUE(location_id,provider_id),
 CHECK(state NOT IN ('paid','failed') OR source_cipher IS NULL),
 CHECK(state<>'paid' OR (paid_at IS NOT NULL AND provider_id IS NOT NULL))
);
CREATE UNIQUE INDEX payment_attempt_active ON public.payment_attempts(checkout_id) WHERE state IN ('submitting','pending','unknown');
CREATE TABLE public.payment_allocations (
 attempt_id text NOT NULL REFERENCES public.payment_attempts(id),
 variant_id bigint NOT NULL,
 physical_quantity bigint NOT NULL CHECK(physical_quantity BETWEEN 0 AND 3960),
 production_quantity bigint NOT NULL CHECK(production_quantity BETWEEN 0 AND 3960),
 PRIMARY KEY(attempt_id,variant_id),
 CHECK(physical_quantity+production_quantity BETWEEN 1 AND 3960)
);
CREATE TABLE public.order_notifications (
 attempt_id text NOT NULL REFERENCES public.payment_attempts(id),
 kind text NOT NULL CHECK(kind IN ('customer_receipt','owner_notification')),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','sending','sent','failed')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(attempt_id,kind)
);
CREATE FUNCTION public.protect_payment_attempt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.id,NEW.shop_id,NEW.basket_digest,NEW.revision,NEW.idempotency_key,NEW.location_id,NEW.quote_hash,NEW.customer,NEW.snapshot,NEW.total_pence,NEW.source_digest,NEW.source_expires_at,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.id,OLD.shop_id,OLD.basket_digest,OLD.revision,OLD.idempotency_key,OLD.location_id,OLD.quote_hash,OLD.customer,OLD.snapshot,OLD.total_pence,OLD.source_digest,OLD.source_expires_at,OLD.created_at)
 OR (NEW.checkout_id IS DISTINCT FROM OLD.checkout_id AND NOT (NEW.checkout_id IS NULL AND OLD.state IN ('paid','failed')))
 OR (OLD.provider_id IS NOT NULL AND NEW.provider_id IS DISTINCT FROM OLD.provider_id)
 OR (NEW.source_cipher IS DISTINCT FROM OLD.source_cipher AND NOT (NEW.source_cipher IS NULL AND (NEW.state IN ('paid','failed') OR OLD.source_expires_at<=clock_timestamp())))
 OR (OLD.state IN ('paid','failed') AND ROW(NEW.state,NEW.paid_at) IS DISTINCT FROM ROW(OLD.state,OLD.paid_at)) THEN
  RAISE EXCEPTION 'payment intent is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER payment_attempt_immutable BEFORE UPDATE ON public.payment_attempts FOR EACH ROW EXECUTE FUNCTION public.protect_payment_attempt();
