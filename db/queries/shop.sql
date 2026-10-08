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
