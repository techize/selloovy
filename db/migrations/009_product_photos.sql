CREATE TABLE public.product_photos (
 id text PRIMARY KEY CHECK(id ~ '^[0-9a-f]{32}$'),
 product_id bigint NOT NULL REFERENCES public.products(id),
 content bytea NOT NULL CHECK(octet_length(content) BETWEEN 1 AND 5242880),
 media_type text NOT NULL CHECK(media_type IN ('image/jpeg','image/png')),
 alt text NOT NULL CHECK(char_length(alt) BETWEEN 1 AND 160),
 width integer NOT NULL CHECK(width BETWEEN 1 AND 4096),
 height integer NOT NULL CHECK(height BETWEEN 1 AND 4096),
 CHECK(width::bigint*height<=8000000)
);
ALTER TABLE public.products ADD COLUMN photo_id text REFERENCES public.product_photos(id);
ALTER TABLE public.product_publications ADD COLUMN photo_id text REFERENCES public.product_photos(id);
