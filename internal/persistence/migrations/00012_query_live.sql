-- +goose Up
-- The single row holds the random secret that signs Message page tokens; it is created by the
-- first query of the installation and never rotated by the product.
CREATE TABLE query_token_keys (
    id smallint PRIMARY KEY CHECK (id = 1),
    secret bytea NOT NULL CHECK (octet_length(secret) = 32)
);
-- A personal view belongs to exactly one Principal; a global view to none.
CREATE TABLE saved_views (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    ownership varchar NOT NULL CHECK (ownership IN ('personal', 'global')),
    owner_principal_id uuid,
    name varchar(256) NOT NULL CHECK (name <> ''),
    definition jsonb NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK ((ownership = 'personal') = (owner_principal_id IS NOT NULL))
);
CREATE INDEX saved_views_owner ON saved_views (owner_principal_id);
-- Fixed one-minute admission windows of search requests, keyed by a hashed bucket.
CREATE TABLE search_admissions (
    bucket bytea NOT NULL CHECK (octet_length(bucket) = 32),
    window_start timestamptz NOT NULL,
    count integer NOT NULL CHECK (count > 0),
    PRIMARY KEY (bucket, window_start)
);

-- +goose Down
DROP TABLE search_admissions;
DROP TABLE saved_views;
DROP TABLE query_token_keys;
