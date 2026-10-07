CREATE TABLE public.owner_auth_limits (
    bucket bytea PRIMARY KEY CHECK (octet_length(bucket) = 32),
    window_start timestamptz NOT NULL,
    attempts integer NOT NULL CHECK (attempts > 0)
);
CREATE INDEX ON public.owner_auth_limits (window_start);
