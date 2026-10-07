CREATE TABLE public.shops (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    currency_code text NOT NULL DEFAULT 'GBP' CHECK (currency_code ~ '^[A-Z]{3}$'),
    country_code text NOT NULL DEFAULT 'GB' CHECK (country_code ~ '^[A-Z]{2}$'),
    timezone text NOT NULL DEFAULT 'Europe/London',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Forward-only: future changes are new migrations, never edits to applied files.
