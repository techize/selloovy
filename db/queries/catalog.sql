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
