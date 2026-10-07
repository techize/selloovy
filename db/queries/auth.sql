-- name: CreateOwner :one
INSERT INTO public.owners (shop_id,email,password_hash) VALUES ($1,$2,$3) RETURNING id;

-- name: SaveMFASeed :exec
UPDATE public.owners SET mfa_ciphertext=$2 WHERE id=$1;

-- name: LockEnrollment :one
SELECT mfa_ciphertext,mfa_enabled FROM public.owners WHERE id=$1 FOR UPDATE;

-- name: DatabaseTime :one
SELECT clock_timestamp()::timestamptz;

-- name: SaveRecoveryCode :exec
INSERT INTO public.owner_recovery_codes (owner_id,digest) VALUES ($1,$2);

-- name: EnableMFA :exec
UPDATE public.owners SET mfa_enabled=true,last_totp_counter=$2 WHERE id=$1;

-- name: OwnerByEmail :one
SELECT id,password_hash,mfa_enabled,auth_version FROM public.owners WHERE email=$1;

-- name: LockPassword :one
SELECT password_hash,auth_version,mfa_enabled FROM public.owners WHERE id=$1 FOR UPDATE;

-- name: CreateChallenge :exec
INSERT INTO public.owner_login_challenges (digest,owner_id,auth_version,expires_at) VALUES ($1,$2,$3,clock_timestamp()+interval '5 minutes');

-- name: ChallengeOwner :one
SELECT owner_id FROM public.owner_login_challenges WHERE digest=$1;

-- name: LockFactor :one
SELECT mfa_ciphertext,mfa_enabled,last_totp_counter,auth_version FROM public.owners WHERE id=$1 FOR UPDATE;

-- name: LockChallenge :one
SELECT consumed_at IS NULL AND attempts<5 AND expires_at>clock_timestamp() AND auth_version=$2 AND purpose='login' AS usable FROM public.owner_login_challenges WHERE digest=$1 FOR UPDATE;

-- name: ConsumeRecoveryCode :execrows
DELETE FROM public.owner_recovery_codes WHERE owner_id=$1 AND digest=$2;

-- name: ConsumeTOTP :exec
UPDATE public.owners SET last_totp_counter=$2 WHERE id=$1;

-- name: FailChallenge :exec
UPDATE public.owner_login_challenges SET attempts=attempts+1 WHERE digest=$1;

-- name: CreateSession :exec
INSERT INTO public.owner_sessions (digest,owner_id,auth_version,created_at,last_seen_at,expires_at) VALUES ($1,$2,$3,sqlc.arg(issued_at)::timestamptz,sqlc.arg(issued_at)::timestamptz,sqlc.arg(issued_at)::timestamptz+interval '8 hours');

-- name: ConsumeChallenge :exec
UPDATE public.owner_login_challenges SET consumed_at=$2 WHERE digest=$1;

-- name: TouchSession :one
UPDATE public.owner_sessions AS session SET last_seen_at=clock_timestamp()
 FROM public.owners AS owner WHERE session.digest=$1 AND owner.id=session.owner_id
 AND owner.mfa_enabled AND owner.auth_version=session.auth_version
 AND session.expires_at>clock_timestamp() AND session.last_seen_at>clock_timestamp()-interval '30 minutes'
 RETURNING session.owner_id;

-- name: DeleteSession :exec
DELETE FROM public.owner_sessions WHERE digest=$1;

-- name: TakeAuthAttempt :one
INSERT INTO public.owner_auth_limits (bucket,window_start,attempts)
VALUES ($1,clock_timestamp(),1)
ON CONFLICT (bucket) DO UPDATE SET
 attempts=CASE WHEN owner_auth_limits.window_start <= clock_timestamp()-interval '15 minutes' THEN 1 ELSE LEAST(owner_auth_limits.attempts+1,1000) END,
 window_start=CASE WHEN owner_auth_limits.window_start <= clock_timestamp()-interval '15 minutes' THEN clock_timestamp() ELSE owner_auth_limits.window_start END
RETURNING attempts;

-- name: LockOwnerSetup :exec
LOCK TABLE public.owners IN EXCLUSIVE MODE;

-- name: OwnerCount :one
SELECT count(*) FROM public.owners;

-- name: CreateSetupShop :one
INSERT INTO public.shops (name) VALUES ($1) RETURNING id;

-- name: PendingOwnerByEmail :one
SELECT id,password_hash,mfa_ciphertext,mfa_enabled FROM public.owners WHERE email=$1;

-- name: DeleteChallenge :exec
DELETE FROM public.owner_login_challenges WHERE digest = $1;
