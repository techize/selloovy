-- name: LockQueuedNotification :one
SELECT n.attempt_id,n.kind,a.customer,a.snapshot,a.paid_at,o.email AS owner_email
FROM public.order_notifications n
JOIN public.payment_attempts a ON a.id=n.attempt_id
JOIN public.owners o ON o.shop_id=a.shop_id
WHERE n.state='queued' AND a.state='paid'
AND n.created_at >= $1::timestamptz AND a.paid_at >= $1::timestamptz
ORDER BY n.created_at,n.attempt_id,n.kind
LIMIT 1 FOR UPDATE OF n SKIP LOCKED;

-- name: StartNotification :execrows
UPDATE public.order_notifications SET state='sending',submission_key=$3,payload=$4,started_at=clock_timestamp()
WHERE attempt_id=$1 AND kind=$2 AND state='queued';

-- name: FinishNotification :execrows
UPDATE public.order_notifications SET state=$3,message_id=$4,finished_at=clock_timestamp()
WHERE attempt_id=$1 AND kind=$2 AND state='sending' AND submission_key=$5;

-- name: ExpireSendingNotifications :execrows
UPDATE public.order_notifications SET state='unknown',finished_at=clock_timestamp()
WHERE (attempt_id,kind) IN (SELECT attempt_id,kind FROM public.order_notifications WHERE state='sending' AND started_at<clock_timestamp()-interval '2 minutes' LIMIT 100 FOR UPDATE SKIP LOCKED);
