ALTER TABLE public.shops ADD COLUMN page_drafts jsonb NOT NULL DEFAULT '[]', ADD COLUMN page_publications jsonb NOT NULL DEFAULT '[]';
ALTER TABLE public.shops ADD CONSTRAINT bounded_pages CHECK (
 jsonb_typeof(page_drafts)='array' AND jsonb_array_length(page_drafts)<=20 AND octet_length(page_drafts::text)<=262144
 AND jsonb_typeof(page_publications)='array' AND jsonb_array_length(page_publications)<=20 AND octet_length(page_publications::text)<=262144
);
