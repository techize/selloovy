ALTER TABLE public.shops ADD COLUMN shipping_services jsonb NOT NULL DEFAULT '[]'
 CHECK(jsonb_typeof(shipping_services)='array' AND jsonb_array_length(shipping_services)<=10 AND octet_length(shipping_services::text)<=16384);
ALTER TABLE public.baskets ADD COLUMN shipping_choice jsonb NOT NULL DEFAULT '{}'
 CHECK(jsonb_typeof(shipping_choice)='object' AND octet_length(shipping_choice::text)<=2048);
ALTER TABLE public.products
 ADD COLUMN preparation_days_min integer NOT NULL DEFAULT 5 CHECK(preparation_days_min BETWEEN 1 AND 90),
 ADD COLUMN preparation_days_max integer NOT NULL DEFAULT 7 CHECK(preparation_days_max BETWEEN 1 AND 90),
 ADD CHECK(preparation_days_min<=preparation_days_max);
