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
SELECT p.id,p.name,p.price_pence,p.revision,p.made_to_order_fallback,p.preparation_days_min,p.preparation_days_max
FROM public.products p JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND p.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: ListMakerVariants :many
SELECT v.id,v.label,v.size_label,v.colour_pair,v.price_pence,v.supply_mode,v.stock_quantity
FROM public.product_variants v JOIN public.products p ON p.id=v.product_id JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND p.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes'
ORDER BY v.id LIMIT 51;

-- name: SaveMakerPolicy :one
UPDATE public.products p SET made_to_order_fallback=$3,revision=p.revision+1,preparation_days_min=COALESCE(sqlc.narg(preparation_days_min),p.preparation_days_min),preparation_days_max=COALESCE(sqlc.narg(preparation_days_max),p.preparation_days_max)
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
INSERT INTO public.product_publications(product_id,revision,snapshot,photo_id) VALUES($1,$2,$3,sqlc.narg(photo_id))
ON CONFLICT(product_id) DO UPDATE SET revision=excluded.revision,snapshot=excluded.snapshot,photo_id=excluded.photo_id;

-- name: UnpublishProduct :exec
DELETE FROM public.product_publications WHERE product_id=$1;

-- name: PublicShop :one
SELECT sp.name,sp.tagline,sp.description FROM public.shop_publications sp
WHERE sp.public_key=$1 AND EXISTS(SELECT 1 FROM public.products p JOIN public.product_publications pp ON pp.product_id=p.id WHERE p.shop_id=sp.shop_id);

-- name: PublicProducts :many
SELECT pp.snapshot,p.id FROM public.product_publications pp JOIN public.products p ON p.id=pp.product_id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE sp.public_key=$1 AND p.id>sqlc.arg(after_id) ORDER BY p.id LIMIT 51;

-- name: PublicProduct :one
SELECT pp.snapshot,p.made_to_order_fallback,p.preparation_days_min,p.preparation_days_max FROM public.product_publications pp JOIN public.products p ON p.id=pp.product_id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE sp.public_key=$1 AND p.id=$2;

-- name: PublicVariantStock :many
SELECT v.id,v.supply_mode,v.stock_quantity FROM public.product_variants v JOIN public.products p ON p.id=v.product_id JOIN public.product_publications pp ON pp.product_id=p.id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE sp.public_key=$1 AND p.id=$2 ORDER BY v.id LIMIT 51;

-- name: AdvancePublicationRevision :one
UPDATE public.products p SET revision=p.revision+1
FROM public.owners o WHERE o.shop_id=p.shop_id AND o.id=$1 AND p.id=$2 AND p.revision=$3
RETURNING p.revision;

-- name: ReadPhoto :one
SELECT p.revision,COALESCE(f.id,'')::text AS photo_id,COALESCE(f.alt,'')::text AS alt,COALESCE(f.width,0)::integer AS width,COALESCE(f.height,0)::integer AS height
FROM public.products p JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
LEFT JOIN public.product_photos f ON f.id=p.photo_id AND f.product_id=p.id
WHERE s.digest=$1 AND p.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: ReadPrivatePhotoContent :one
SELECT f.content,f.media_type FROM public.product_photos f JOIN public.products p ON p.id=f.product_id AND p.photo_id=f.id JOIN public.owners o ON o.shop_id=p.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND p.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: AddPhoto :exec
INSERT INTO public.product_photos(id,product_id,content,media_type,alt,width,height) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: SelectPhoto :one
UPDATE public.products p SET photo_id=sqlc.narg(photo_id),revision=p.revision+1
FROM public.owners o WHERE o.shop_id=p.shop_id AND o.id=sqlc.arg(owner_id) AND p.id=sqlc.arg(product_id) AND p.revision=sqlc.arg(revision)
RETURNING p.revision;

-- name: DeleteUnusedPhotos :exec
DELETE FROM public.product_photos f WHERE f.product_id=$1
AND NOT EXISTS(SELECT 1 FROM public.products p WHERE p.photo_id=f.id)
AND NOT EXISTS(SELECT 1 FROM public.product_publications pp WHERE pp.photo_id=f.id);

-- name: PublicPhotoContent :one
SELECT f.content,f.media_type FROM public.product_photos f JOIN public.products p ON p.id=f.product_id JOIN public.product_publications pp ON pp.product_id=p.id AND pp.photo_id=f.id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE sp.public_key=$1 AND p.id=$2 AND f.id=$3;

-- name: ReadBasket :one
SELECT b.revision,b.lines,b.shipping_choice FROM public.baskets b JOIN public.shop_publications sp ON sp.shop_id=b.shop_id
WHERE sp.public_key=$1 AND b.digest=$2 AND b.expires_at>clock_timestamp();

-- name: CreateBasket :exec
INSERT INTO public.baskets(shop_id,digest)
SELECT shop_id,$2 FROM public.shop_publications WHERE public_key=$1
ON CONFLICT(shop_id,digest) DO NOTHING;

-- name: LockBasket :one
SELECT b.revision,b.lines,b.shipping_choice FROM public.baskets b JOIN public.shop_publications sp ON sp.shop_id=b.shop_id
WHERE sp.public_key=$1 AND b.digest=$2 AND b.expires_at>clock_timestamp() FOR UPDATE OF b;

-- name: SaveBasket :execrows
UPDATE public.baskets b SET lines=$3,revision=b.revision+1,shipping_choice=$5 FROM public.shop_publications sp
WHERE sp.shop_id=b.shop_id AND sp.public_key=$1 AND b.digest=$2 AND b.revision=$4 AND b.expires_at>clock_timestamp();

-- name: PurgeExpiredBaskets :exec
DELETE FROM public.baskets WHERE (shop_id,digest) IN (
SELECT b.shop_id,b.digest FROM public.baskets b JOIN public.shop_publications sp ON sp.shop_id=b.shop_id
WHERE sp.public_key=$1 AND b.expires_at<=clock_timestamp() ORDER BY b.expires_at LIMIT 1000
);

-- name: PublicShipping :one
SELECT sh.revision,sh.shipping_services FROM public.shops sh JOIN public.shop_publications sp ON sp.shop_id=sh.id WHERE sp.public_key=$1;
