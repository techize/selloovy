CREATE TABLE public.baskets (
 shop_id bigint NOT NULL REFERENCES public.shops(id),
 digest bytea NOT NULL CHECK(octet_length(digest)=32),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 lines jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(lines)='array' AND jsonb_array_length(lines)<=40 AND octet_length(lines::text)<=131072),
 expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '7 days',
 PRIMARY KEY(shop_id,digest)
);
CREATE INDEX baskets_expiry ON public.baskets(expires_at);
