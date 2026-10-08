CREATE TABLE public.products (
 id bigserial PRIMARY KEY,
 shop_id bigint NOT NULL REFERENCES public.shops(id),
 name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160),
 description text NOT NULL DEFAULT '' CHECK (char_length(description)<=5000),
 price_pence bigint NOT NULL CHECK (price_pence BETWEEN 1 AND 100000000),
 certificate_name text NOT NULL DEFAULT 'none' CHECK (certificate_name IN ('none','optional','required')),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
 creation_key text NOT NULL CHECK (char_length(creation_key)=36),
 creation_hash bytea NOT NULL CHECK (octet_length(creation_hash)=32),
 UNIQUE(shop_id,creation_key)
);
CREATE INDEX products_shop_listing ON public.products(shop_id,id);
