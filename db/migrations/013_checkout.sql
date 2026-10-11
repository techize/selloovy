CREATE TABLE public.checkout_reviews (
 id text PRIMARY KEY CHECK(id ~ '^[0-9a-f]{32}$'),
 shop_id bigint NOT NULL,
 basket_digest bytea NOT NULL,
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 customer jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(customer)='object' AND octet_length(customer::text)<=8192),
 snapshot jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(snapshot)='object' AND octet_length(snapshot::text)<=131072),
 quote_hash text NOT NULL DEFAULT '',
 expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '30 days',
 hold_expires_at timestamptz,
 payment_pending boolean NOT NULL DEFAULT false,
 UNIQUE(shop_id,basket_digest),
 FOREIGN KEY(shop_id,basket_digest) REFERENCES public.baskets(shop_id,digest) ON DELETE CASCADE
);
CREATE TABLE public.checkout_stock_holds (
 checkout_id text NOT NULL REFERENCES public.checkout_reviews(id) ON DELETE CASCADE,
 variant_id bigint NOT NULL REFERENCES public.product_variants(id),
 quantity bigint NOT NULL CHECK(quantity BETWEEN 1 AND 3960),
 expires_at timestamptz NOT NULL,
 retained boolean NOT NULL DEFAULT false,
 PRIMARY KEY(checkout_id,variant_id)
);
CREATE INDEX checkout_holds_variant ON public.checkout_stock_holds(variant_id);
-- Customer activity extends unpaid recovery. Unresolved payments must not be purged.
ALTER TABLE public.baskets ALTER COLUMN expires_at SET DEFAULT clock_timestamp()+interval '30 days';
