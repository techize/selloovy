-- name: ListProducts :many
SELECT p.id,p.name,p.description,p.price_pence,p.certificate_name,p.revision
FROM public.products p JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes'
AND p.id>sqlc.arg(after_id) ORDER BY p.id LIMIT 51;

-- name: ReadProduct :one
SELECT p.id,p.name,p.description,p.price_pence,p.certificate_name,p.revision
FROM public.products p JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND p.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: FindCreation :one
SELECT p.id,p.creation_hash FROM public.products p JOIN public.owners o ON o.shop_id=p.shop_id
WHERE o.id=$1 AND p.creation_key=$2;

-- name: CreateProduct :one
INSERT INTO public.products(shop_id,name,description,price_pence,certificate_name,creation_key,creation_hash)
SELECT o.shop_id,$2,$3,$4,$5,$6,$7 FROM public.owners o JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes'
RETURNING id,name,description,price_pence,certificate_name,revision;

-- name: UpdateProduct :one
UPDATE public.products p SET name=$3,description=$4,price_pence=$5,certificate_name=$6,revision=p.revision+1
FROM public.owners o JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE p.shop_id=o.shop_id AND s.digest=$1 AND p.id=$2 AND p.revision=$7
AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes'
RETURNING p.id,p.name,p.description,p.price_pence,p.certificate_name,p.revision;

-- name: ReadMakerProduct :one
SELECT p.id,p.name,p.price_pence,p.revision,p.made_to_order_fallback
FROM public.products p JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND p.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: ListMakerVariants :many
SELECT v.id,v.label,v.size_label,v.colour_pair,v.price_pence,v.supply_mode,v.stock_quantity
FROM public.product_variants v JOIN public.products p ON p.id=v.product_id JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND p.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes'
ORDER BY v.id LIMIT 51;

-- name: SaveMakerPolicy :one
UPDATE public.products p SET made_to_order_fallback=$3,revision=p.revision+1
FROM public.owners o JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE p.shop_id=o.shop_id AND s.digest=$1 AND p.id=$2 AND p.revision=$4
AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes'
RETURNING p.id;

-- name: AddMakerVariant :one
INSERT INTO public.product_variants(product_id,label,label_key,size_label,colour_pair,price_pence,supply_mode,stock_quantity)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id;

-- name: UpdateMakerVariant :execrows
UPDATE public.product_variants SET label=$3,label_key=$4,size_label=$5,colour_pair=$6,price_pence=$7,supply_mode=$8,stock_quantity=$9 WHERE product_id=$1 AND id=$2;

-- name: RecordStockAdjustment :exec
INSERT INTO public.stock_adjustments(variant_id,owner_id,previous_quantity,new_quantity,reason) VALUES($1,$2,$3,$4,$5);

-- name: ReadPublication :one
SELECT p.revision,COALESCE(pp.revision,0)::bigint AS published_revision,COALESCE(sp.public_key,'')::text AS public_key,sh.name AS shop_name,sh.tagline AS shop_tagline,sh.description AS shop_description,sh.revision AS shop_revision
FROM public.products p JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
JOIN public.shops sh ON sh.id=p.shop_id LEFT JOIN public.product_publications pp ON pp.product_id=p.id LEFT JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE s.digest=$1 AND p.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: PublishShop :one
INSERT INTO public.shop_publications(shop_id,public_key,name,tagline,description)
SELECT sh.id,$2,sh.name,sh.tagline,sh.description FROM public.shops sh JOIN public.owners o ON o.shop_id=sh.id WHERE o.id=$1
ON CONFLICT(shop_id) DO UPDATE SET name=excluded.name,tagline=excluded.tagline,description=excluded.description
RETURNING public_key;

-- name: PublishProduct :exec
INSERT INTO public.product_publications(product_id,revision,snapshot) VALUES($1,$2,$3)
ON CONFLICT(product_id) DO UPDATE SET revision=excluded.revision,snapshot=excluded.snapshot;

-- name: UnpublishProduct :exec
DELETE FROM public.product_publications WHERE product_id=$1;

-- name: PublicShop :one
SELECT sp.name,sp.tagline,sp.description FROM public.shop_publications sp
WHERE sp.public_key=$1 AND EXISTS(SELECT 1 FROM public.products p JOIN public.product_publications pp ON pp.product_id=p.id WHERE p.shop_id=sp.shop_id);

-- name: PublicProducts :many
SELECT pp.snapshot,p.id FROM public.product_publications pp JOIN public.products p ON p.id=pp.product_id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE sp.public_key=$1 AND p.id>sqlc.arg(after_id) ORDER BY p.id LIMIT 51;

-- name: PublicProduct :one
SELECT pp.snapshot,p.made_to_order_fallback FROM public.product_publications pp JOIN public.products p ON p.id=pp.product_id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE sp.public_key=$1 AND p.id=$2;

-- name: PublicVariantStock :many
SELECT v.id,v.supply_mode,v.stock_quantity FROM public.product_variants v JOIN public.products p ON p.id=v.product_id JOIN public.product_publications pp ON pp.product_id=p.id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE sp.public_key=$1 AND p.id=$2 ORDER BY v.id LIMIT 51;

-- name: AdvancePublicationRevision :one
UPDATE public.products p SET revision=p.revision+1
FROM public.owners o WHERE o.shop_id=p.shop_id AND o.id=$1 AND p.id=$2 AND p.revision=$3
RETURNING p.revision;
