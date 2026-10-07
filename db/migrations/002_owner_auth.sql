CREATE TABLE public.owners (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    shop_id bigint NOT NULL UNIQUE REFERENCES public.shops(id),
    email text NOT NULL UNIQUE CHECK (char_length(email) BETWEEN 3 AND 254 AND email = lower(email)),
    password_hash text NOT NULL,
    mfa_ciphertext bytea,
    mfa_enabled boolean NOT NULL DEFAULT false,
    last_totp_counter bigint NOT NULL DEFAULT -1 CHECK (last_totp_counter >= -1),
    auth_version bigint NOT NULL DEFAULT 1 CHECK (auth_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT mfa_enabled OR mfa_ciphertext IS NOT NULL)
);
CREATE TABLE public.owner_recovery_codes (
    owner_id bigint NOT NULL REFERENCES public.owners(id) ON DELETE CASCADE,
    digest bytea NOT NULL CHECK (octet_length(digest) = 32),
    PRIMARY KEY (owner_id, digest)
);
CREATE TABLE public.owner_login_challenges (
    digest bytea PRIMARY KEY CHECK (octet_length(digest) = 32),
    owner_id bigint NOT NULL REFERENCES public.owners(id) ON DELETE CASCADE,
    auth_version bigint NOT NULL,
    purpose text NOT NULL DEFAULT 'login' CHECK (purpose = 'login'),
    expires_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    consumed_at timestamptz
);
CREATE INDEX ON public.owner_login_challenges (expires_at);
CREATE TABLE public.owner_sessions (
    digest bytea PRIMARY KEY CHECK (octet_length(digest) = 32),
    owner_id bigint NOT NULL REFERENCES public.owners(id) ON DELETE CASCADE,
    auth_version bigint NOT NULL,
    created_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX ON public.owner_sessions (expires_at);
