ALTER TABLE public.products ADD COLUMN made_to_order_fallback boolean NOT NULL DEFAULT false;
CREATE TABLE public.product_variants (
 id bigserial PRIMARY KEY,
 product_id bigint NOT NULL REFERENCES public.products(id),
 label text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 120),
 label_key text NOT NULL CHECK (char_length(label_key) BETWEEN 1 AND 240),
 size_label text NOT NULL DEFAULT '' CHECK (char_length(size_label)<=80),
 colour_pair text NOT NULL DEFAULT '' CHECK (char_length(colour_pair)<=120),
 price_pence bigint CHECK (price_pence BETWEEN 1 AND 100000000),
 supply_mode text NOT NULL CHECK (supply_mode IN ('stocked','made_to_order')),
 stock_quantity bigint NOT NULL DEFAULT 0 CHECK (stock_quantity BETWEEN 0 AND 1000000),
 CHECK (supply_mode='stocked' OR stock_quantity=0),
 UNIQUE(product_id,label_key) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX product_variants_listing ON public.product_variants(product_id,id);
CREATE TABLE public.stock_adjustments (
 id bigserial PRIMARY KEY,
 variant_id bigint NOT NULL REFERENCES public.product_variants(id),
 owner_id bigint NOT NULL REFERENCES public.owners(id),
 previous_quantity bigint NOT NULL CHECK (previous_quantity BETWEEN 0 AND 1000000),
 new_quantity bigint NOT NULL CHECK (new_quantity BETWEEN 0 AND 1000000),
 reason text NOT NULL CHECK (reason IN ('initial_stock','admin_count')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX stock_adjustments_variant ON public.stock_adjustments(variant_id,id);
