-- name: ReadShop :one
SELECT shop.name,shop.tagline,shop.description,shop.contact_email,shop.currency_code,shop.country_code,shop.timezone,shop.revision
FROM public.shops shop JOIN public.owners owner ON owner.shop_id=shop.id
JOIN public.owner_sessions session ON session.owner_id=owner.id
WHERE session.digest=$1 AND session.auth_version=owner.auth_version
AND session.expires_at>clock_timestamp() AND session.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: LockShopSession :one
SELECT owner.id FROM public.owners owner JOIN public.owner_sessions session ON session.owner_id=owner.id
WHERE session.digest=$1 AND session.auth_version=owner.auth_version
AND session.expires_at>clock_timestamp() AND session.last_seen_at>clock_timestamp()-interval '30 minutes'
FOR UPDATE OF owner,session;

-- name: LockShopRevision :one
SELECT shop.revision FROM public.shops shop JOIN public.owners owner ON owner.shop_id=shop.id WHERE owner.id=$1 FOR UPDATE OF shop;

-- name: SaveShop :one
UPDATE public.shops shop SET name=$2,tagline=$3,description=$4,contact_email=$5,revision=shop.revision+1
FROM public.owners owner JOIN public.owner_sessions session ON session.owner_id=owner.id
WHERE shop.id=owner.shop_id AND session.digest=$1 AND session.auth_version=owner.auth_version
AND session.expires_at>clock_timestamp() AND session.last_seen_at>clock_timestamp()-interval '30 minutes'
RETURNING shop.name,shop.tagline,shop.description,shop.contact_email,shop.currency_code,shop.country_code,shop.timezone,shop.revision;

-- name: ReadShipping :one
SELECT sh.revision,sh.shipping_services FROM public.shops sh JOIN public.owners o ON o.shop_id=sh.id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: SaveShipping :one
UPDATE public.shops sh SET shipping_services=$2,revision=sh.revision+1 FROM public.owners o
WHERE o.id=$1 AND o.shop_id=sh.id RETURNING sh.revision,sh.shipping_services;

-- name: ReadPages :one
SELECT sh.revision,sh.page_drafts,sh.page_publications,COALESCE(p.public_key,'')::text AS public_key
FROM public.shops sh JOIN public.owners o ON o.shop_id=sh.id JOIN public.owner_sessions s ON s.owner_id=o.id
LEFT JOIN public.shop_publications p ON p.shop_id=sh.id
WHERE s.digest=$1 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: SavePages :one
UPDATE public.shops sh SET page_drafts=$2,page_publications=$3,revision=sh.revision+1 FROM public.owners o
WHERE o.id=$1 AND o.shop_id=sh.id RETURNING sh.revision;

-- name: PublicPages :one
SELECT sh.page_publications FROM public.shops sh JOIN public.shop_publications p ON p.shop_id=sh.id WHERE p.public_key=$1;

-- name: PublishedPageProduct :one
SELECT pp.product_id FROM public.product_publications pp JOIN public.products p ON p.id=pp.product_id JOIN public.owners o ON o.shop_id=p.shop_id WHERE o.id=$1 AND p.id=$2;

-- name: PageProducts :many
SELECT pp.product_id,pp.snapshot,pp.photo_id FROM public.product_publications pp
JOIN public.products p ON p.id=pp.product_id JOIN public.owners o ON o.shop_id=p.shop_id
JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes'
ORDER BY pp.product_id LIMIT 501;
