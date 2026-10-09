-- Public projections are created only by an authenticated, explicit publish action.
CREATE TABLE public.shop_publications (
 shop_id bigint PRIMARY KEY REFERENCES public.shops(id),
 public_key text NOT NULL UNIQUE CHECK(public_key ~ '^[0-9a-f]{32}$'),
 name text NOT NULL,
 tagline text NOT NULL,
 description text NOT NULL
);
CREATE TABLE public.product_publications (
 product_id bigint PRIMARY KEY REFERENCES public.products(id),
 revision bigint NOT NULL CHECK(revision>0),
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object')
);
