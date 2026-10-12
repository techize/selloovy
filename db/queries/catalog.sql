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
SELECT v.id,v.label,v.size_label,v.colour_pair,v.price_pence,v.supply_mode,v.stock_quantity,COALESCE((SELECT sum(h.quantity) FROM public.checkout_stock_holds h WHERE h.variant_id=v.id AND (h.retained OR h.expires_at>clock_timestamp())),0)::bigint AS allocated_quantity
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
SELECT v.id,v.supply_mode,GREATEST(0,v.stock_quantity-COALESCE((SELECT sum(h.quantity) FROM public.checkout_stock_holds h WHERE h.variant_id=v.id AND (h.retained OR h.expires_at>clock_timestamp()) AND h.checkout_id<>COALESCE(sqlc.narg(exclude_checkout)::text,'')),0))::bigint AS stock_quantity FROM public.product_variants v JOIN public.products p ON p.id=v.product_id JOIN public.product_publications pp ON pp.product_id=p.id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
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
UPDATE public.baskets b SET lines=$3,revision=b.revision+1,shipping_choice=$5,expires_at=clock_timestamp()+interval '30 days' FROM public.shop_publications sp
WHERE sp.shop_id=b.shop_id AND sp.public_key=$1 AND b.digest=$2 AND b.revision=$4 AND b.expires_at>clock_timestamp();

-- name: PurgeExpiredBaskets :exec
DELETE FROM public.baskets WHERE (shop_id,digest) IN (
SELECT b.shop_id,b.digest FROM public.baskets b JOIN public.shop_publications sp ON sp.shop_id=b.shop_id
WHERE sp.public_key=$1 AND b.expires_at<=clock_timestamp() AND NOT EXISTS(SELECT 1 FROM public.checkout_reviews c WHERE c.shop_id=b.shop_id AND c.basket_digest=b.digest AND c.payment_pending) ORDER BY b.expires_at LIMIT 1000
);

-- name: PublicShipping :one
SELECT sh.revision,sh.shipping_services FROM public.shops sh JOIN public.shop_publications sp ON sp.shop_id=sh.id WHERE sp.public_key=$1;

-- name: BasketShop :one
SELECT sp.name,sp.tagline,sp.description FROM public.shop_publications sp WHERE sp.public_key=$1;

-- name: ReadCheckout :one
SELECT c.id,c.revision,c.customer,c.snapshot,c.quote_hash,c.hold_expires_at,c.payment_pending,c.completed,COALESCE((SELECT a.id FROM public.payment_attempts a WHERE a.checkout_id=c.id AND a.state IN ('paid','submitting','pending','unknown') ORDER BY a.created_at DESC LIMIT 1),'')::text AS payment_id FROM public.checkout_reviews c JOIN public.shop_publications sp ON sp.shop_id=c.shop_id JOIN public.baskets b ON b.shop_id=c.shop_id AND b.digest=c.basket_digest
WHERE sp.public_key=$1 AND c.basket_digest=$2 AND (c.expires_at>clock_timestamp() OR c.payment_pending) AND (b.expires_at>clock_timestamp() OR c.payment_pending);

-- name: CreateCheckout :exec
INSERT INTO public.checkout_reviews(id,shop_id,basket_digest)
SELECT $3,b.shop_id,b.digest FROM public.baskets b JOIN public.shop_publications sp ON sp.shop_id=b.shop_id
WHERE sp.public_key=$1 AND b.digest=$2 AND b.expires_at>clock_timestamp()
ON CONFLICT(shop_id,basket_digest) DO NOTHING;

-- name: LockCheckout :one
SELECT c.id,c.revision,c.customer,c.snapshot,c.quote_hash,c.hold_expires_at,c.payment_pending,c.completed,COALESCE((SELECT a.id FROM public.payment_attempts a WHERE a.checkout_id=c.id AND a.state IN ('paid','submitting','pending','unknown') ORDER BY a.created_at DESC LIMIT 1),'')::text AS payment_id FROM public.checkout_reviews c JOIN public.shop_publications sp ON sp.shop_id=c.shop_id
WHERE sp.public_key=$1 AND c.basket_digest=$2 AND (c.expires_at>clock_timestamp() OR c.payment_pending) FOR UPDATE OF c;

-- name: SaveCheckout :exec
UPDATE public.checkout_reviews SET revision=revision+1,customer=$2,snapshot=$3,quote_hash=$4,expires_at=clock_timestamp()+interval '30 days',hold_expires_at=NULL WHERE id=$1;

-- name: TouchCheckoutBasket :exec
UPDATE public.baskets SET expires_at=clock_timestamp()+interval '30 days' WHERE shop_id=(SELECT shop_id FROM public.shop_publications WHERE public_key=$1) AND digest=$2;

-- name: LockCheckoutShop :one
SELECT sh.id FROM public.shops sh JOIN public.shop_publications sp ON sp.shop_id=sh.id WHERE sp.public_key=$1 FOR SHARE OF sh;

-- name: LockCheckoutProducts :many
SELECT p.id FROM public.products p JOIN public.shop_publications sp ON sp.shop_id=p.shop_id WHERE sp.public_key=$1 AND p.id=ANY(sqlc.arg(product_ids)::bigint[]) ORDER BY p.id FOR SHARE OF p;

-- name: LockCheckoutVariants :many
SELECT v.id,v.product_id,v.supply_mode,v.stock_quantity FROM public.product_variants v JOIN public.products p ON p.id=v.product_id JOIN public.shop_publications sp ON sp.shop_id=p.shop_id
WHERE sp.public_key=$1 AND v.id=ANY(sqlc.arg(variant_ids)::bigint[]) ORDER BY v.id FOR UPDATE OF v;

-- name: HeldQuantity :one
SELECT COALESCE(sum(quantity),0)::bigint FROM public.checkout_stock_holds WHERE variant_id=$1 AND checkout_id<>$2 AND (retained OR expires_at>clock_timestamp());

-- name: ClearCheckoutHolds :exec
DELETE FROM public.checkout_stock_holds WHERE checkout_id=$1 AND NOT retained;

-- name: AddCheckoutHold :exec
INSERT INTO public.checkout_stock_holds(checkout_id,variant_id,quantity,expires_at) VALUES($1,$2,$3,$4);

-- name: SetCheckoutHoldDeadline :one
UPDATE public.checkout_reviews SET hold_expires_at=clock_timestamp()+interval '15 minutes' WHERE id=$1 RETURNING hold_expires_at;

-- name: RetainCheckoutHolds :exec
UPDATE public.checkout_stock_holds SET retained=true WHERE checkout_id=$1;

-- name: MarkCheckoutPending :exec
UPDATE public.checkout_reviews SET payment_pending=true WHERE id=$1;

-- name: CheckoutHoldActive :one
SELECT COALESCE(hold_expires_at>clock_timestamp(),false)::boolean FROM public.checkout_reviews WHERE id=$1;

-- name: PurgeOwnedExpiredCheckout :exec
DELETE FROM public.checkout_reviews WHERE shop_id=(SELECT shop_id FROM public.shop_publications WHERE public_key=$1) AND basket_digest=$2 AND expires_at<=clock_timestamp() AND NOT payment_pending;

-- name: LockMakerStock :many
SELECT v.id FROM public.product_variants v JOIN public.products p ON p.id=v.product_id JOIN public.owners o ON o.shop_id=p.shop_id WHERE o.id=$1 AND p.id=$2 ORDER BY v.id FOR UPDATE OF v;

-- name: ActivePaymentPreparation :one
SELECT *, expires_at>clock_timestamp() AS active FROM public.payment_preparations WHERE checkout_id=$1 AND state='prepared' FOR UPDATE;

-- name: OwnedPaymentPreparation :one
SELECT *, expires_at>clock_timestamp() AS active FROM public.payment_preparations WHERE checkout_id=$1 AND id=$2 FOR UPDATE;

-- name: CreatePaymentPreparation :one
INSERT INTO public.payment_preparations(id,checkout_id,checkout_revision,quote_hash,idempotency_key,customer,snapshot,currency,total_pence,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *, expires_at>clock_timestamp() AS active;

-- name: EndPaymentPreparation :exec
UPDATE public.payment_preparations SET state=$2 WHERE id=$1 AND state='prepared';

-- name: ClearCheckoutHoldDeadline :exec
UPDATE public.checkout_reviews SET hold_expires_at=NULL WHERE id=$1 AND NOT payment_pending;

-- name: ReadPaymentAttempt :one
SELECT * FROM public.payment_attempts WHERE id=$1;

-- name: LockPaymentAttempt :one
SELECT * FROM public.payment_attempts WHERE id=$1 FOR UPDATE;

-- name: CreatePaymentAttempt :one
INSERT INTO public.payment_attempts(id,shop_id,checkout_id,basket_digest,revision,idempotency_key,location_id,quote_hash,customer,snapshot,total_pence,source_digest,source_cipher)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING *;

-- name: CheckoutPhysicalHolds :many
SELECT variant_id,quantity FROM public.checkout_stock_holds WHERE checkout_id=$1 AND (retained OR expires_at>clock_timestamp()) ORDER BY variant_id;

-- name: AddPaymentAllocation :exec
INSERT INTO public.payment_allocations(attempt_id,variant_id,physical_quantity,production_quantity) VALUES($1,$2,$3,$4);

-- name: AttachPaymentProviderID :execrows
UPDATE public.payment_attempts SET provider_id=$2 WHERE id=$1 AND (provider_id IS NULL OR provider_id=$2);

-- name: SetPaymentOutcome :exec
UPDATE public.payment_attempts SET state=$2,paid_at=$3,source_cipher=CASE WHEN $2 IN ('paid','failed') THEN NULL ELSE source_cipher END WHERE id=$1;

-- name: LockPaymentBasket :one
SELECT b.shop_id FROM public.baskets b JOIN public.checkout_reviews c ON c.shop_id=b.shop_id AND c.basket_digest=b.digest WHERE c.id=$1 FOR UPDATE OF b;

-- name: LockPaymentReview :one
SELECT * FROM public.checkout_reviews WHERE id=$1 FOR UPDATE;

-- name: LockPaymentProducts :many
SELECT p.id FROM public.products p JOIN public.product_variants v ON v.product_id=p.id JOIN public.payment_allocations a ON a.variant_id=v.id WHERE a.attempt_id=$1 GROUP BY p.id ORDER BY p.id;

-- name: LockPaymentProduct :one
SELECT id FROM public.products WHERE id=$1 FOR UPDATE;

-- name: LockPaymentVariants :many
SELECT v.id FROM public.product_variants v JOIN public.payment_allocations a ON a.variant_id=v.id WHERE a.attempt_id=$1 ORDER BY v.id FOR UPDATE OF v;

-- name: ConsumePaymentStock :execrows
UPDATE public.product_variants v SET stock_quantity=v.stock_quantity-a.physical_quantity FROM public.payment_allocations a WHERE a.attempt_id=$1 AND a.variant_id=v.id AND a.physical_quantity>0 AND v.stock_quantity>=a.physical_quantity;

-- name: PhysicalPaymentVariantCount :one
SELECT count(*)::bigint FROM public.payment_allocations WHERE attempt_id=$1 AND physical_quantity>0;

-- name: TouchPaidProductRevisions :exec
UPDATE public.products SET revision=revision+1 WHERE id IN (SELECT v.product_id FROM public.product_variants v JOIN public.payment_allocations a ON a.variant_id=v.id WHERE a.attempt_id=$1 AND a.physical_quantity>0);

-- name: ReleaseFinancialHolds :exec
DELETE FROM public.checkout_stock_holds WHERE checkout_id=$1;

-- name: CompletePaidCheckout :exec
UPDATE public.checkout_reviews SET completed=true,payment_pending=false,hold_expires_at=NULL WHERE id=$1;

-- name: FailFinancialCheckout :exec
UPDATE public.checkout_reviews SET payment_pending=false,hold_expires_at=NULL WHERE id=$1 AND NOT completed;

-- name: QueueOrderNotifications :exec
INSERT INTO public.order_notifications(attempt_id,kind) VALUES($1,'customer_receipt'),($1,'owner_notification') ON CONFLICT DO NOTHING;

-- name: BeginFinancialPending :execrows
UPDATE public.checkout_reviews SET payment_pending=true WHERE id=$1 AND NOT payment_pending AND NOT completed AND hold_expires_at>clock_timestamp();

-- name: PaidAttemptForOwnedBasket :one
SELECT a.id FROM public.payment_attempts a JOIN public.shop_publications sp ON sp.shop_id=a.shop_id WHERE sp.public_key=$1 AND a.basket_digest=$2 AND a.state='paid' ORDER BY a.created_at DESC LIMIT 1;

-- name: EraseExpiredPaymentSources :execrows
UPDATE public.payment_attempts SET source_cipher=NULL WHERE id IN (SELECT id FROM public.payment_attempts WHERE source_cipher IS NOT NULL AND source_expires_at<=clock_timestamp() ORDER BY source_expires_at LIMIT 1000 FOR UPDATE SKIP LOCKED);

-- name: TakePaymentBudget :one
INSERT INTO public.payment_attempt_limits(fingerprint,attempts) VALUES(sqlc.arg(fingerprint),1)
ON CONFLICT(fingerprint) DO UPDATE SET
 attempts=CASE WHEN payment_attempt_limits.window_started_at<=clock_timestamp()-interval '15 minutes' THEN 1 ELSE payment_attempt_limits.attempts+1 END,
 window_started_at=CASE WHEN payment_attempt_limits.window_started_at<=clock_timestamp()-interval '15 minutes' THEN clock_timestamp() ELSE payment_attempt_limits.window_started_at END
WHERE payment_attempt_limits.window_started_at<=clock_timestamp()-interval '15 minutes' OR payment_attempt_limits.attempts<sqlc.arg(maximum)::integer
RETURNING attempts;

-- name: PurgePaymentBudgets :execrows
DELETE FROM public.payment_attempt_limits WHERE fingerprint IN (SELECT fingerprint FROM public.payment_attempt_limits WHERE window_started_at<=clock_timestamp()-interval '1 day' ORDER BY window_started_at LIMIT 1000 FOR UPDATE SKIP LOCKED);

-- name: ClaimPaymentReconciliations :many
WITH due AS (SELECT queued.id FROM public.payment_attempts queued WHERE queued.location_id=sqlc.arg(location_id) AND queued.state IN ('submitting','pending','unknown') AND queued.provider_id IS NOT NULL AND queued.reconcile_after<=clock_timestamp() ORDER BY queued.reconcile_after LIMIT 5 FOR UPDATE SKIP LOCKED)
UPDATE public.payment_attempts a SET reconcile_after=clock_timestamp()+interval '1 minute' FROM due WHERE a.id=due.id RETURNING a.id;

-- name: GuestPaymentView :one
SELECT a.* FROM public.payment_attempts a JOIN public.shop_publications sp ON sp.shop_id=a.shop_id
WHERE sp.public_key=$1 AND a.basket_digest=$2 AND a.id=$3 AND a.created_at>clock_timestamp()-interval '30 days';

-- name: GuestPreparedView :one
SELECT p.* FROM public.payment_preparations p JOIN public.checkout_reviews c ON c.id=p.checkout_id JOIN public.shop_publications sp ON sp.shop_id=c.shop_id JOIN public.baskets b ON b.shop_id=c.shop_id AND b.digest=c.basket_digest
WHERE sp.public_key=$1 AND c.basket_digest=$2 AND p.id=$3 AND c.expires_at>clock_timestamp() AND b.expires_at>clock_timestamp() AND p.state='prepared' AND p.expires_at>clock_timestamp() AND NOT c.payment_pending AND NOT c.completed;

-- name: CreateOrderFulfilment :exec
INSERT INTO public.order_fulfilment(attempt_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: OwnedPaymentRecord :one
SELECT a.* FROM public.payment_attempts a JOIN public.owners o ON o.shop_id=a.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND a.id=$2 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: ListOwnedPayments :many
SELECT a.id,a.state,a.customer,a.total_pence,a.created_at,a.paid_at,(a.provider_id IS NOT NULL)::boolean AS provider_known,COALESCE(f.state,'')::text AS fulfilment_state FROM public.payment_attempts a LEFT JOIN public.order_fulfilment f ON f.attempt_id=a.id JOIN public.owners o ON o.shop_id=a.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes'
AND (NOT sqlc.arg(attention_only)::boolean OR (a.state IN ('submitting','pending','unknown') AND a.created_at<clock_timestamp()-interval '5 minutes'))
AND (sqlc.arg(after_id)::text='' OR (a.created_at,a.id)<(SELECT cursor.created_at,cursor.id FROM public.payment_attempts cursor WHERE cursor.id=sqlc.arg(after_id) AND cursor.shop_id=a.shop_id))
ORDER BY a.created_at DESC,a.id DESC LIMIT 51;

-- name: ReadOrderFulfilment :one
SELECT * FROM public.order_fulfilment WHERE attempt_id=$1;

-- name: ReadOrderAllocations :many
SELECT variant_id,physical_quantity,production_quantity FROM public.payment_allocations WHERE attempt_id=$1 ORDER BY variant_id;

-- name: ReadOrderNotifications :many
SELECT kind,state FROM public.order_notifications WHERE attempt_id=$1 ORDER BY kind;

-- name: SaveOrderDispatch :execrows
UPDATE public.order_fulfilment f SET state='dispatched',revision=f.revision+1,carrier=sqlc.arg(carrier),tracking_reference=sqlc.arg(tracking_reference),tracking_url=sqlc.arg(tracking_url),dispatched_at=COALESCE(f.dispatched_at,clock_timestamp())
FROM public.payment_attempts a JOIN public.owners o ON o.shop_id=a.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE f.attempt_id=sqlc.arg(attempt_id) AND f.revision=sqlc.arg(revision) AND a.id=f.attempt_id AND a.state='paid' AND s.digest=sqlc.arg(digest) AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes';

-- name: ListOwnedUnfinishedCheckouts :many
SELECT c.id,c.customer,c.snapshot,(c.expires_at-interval '30 days')::timestamptz AS saved_at
FROM public.checkout_reviews c JOIN public.owners o ON o.shop_id=c.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes' AND c.revision>0 AND NOT c.completed AND NOT c.payment_pending AND c.expires_at>clock_timestamp()
ORDER BY c.expires_at DESC,c.id DESC LIMIT 50;

-- name: OwnedPaymentAttentionCount :one
SELECT count(*)::bigint FROM public.payment_attempts a JOIN public.owners o ON o.shop_id=a.shop_id JOIN public.owner_sessions s ON s.owner_id=o.id
WHERE s.digest=$1 AND s.auth_version=o.auth_version AND s.expires_at>clock_timestamp() AND s.last_seen_at>clock_timestamp()-interval '30 minutes' AND a.state IN ('submitting','pending','unknown') AND a.created_at<clock_timestamp()-interval '5 minutes';
