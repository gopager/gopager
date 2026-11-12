-- +goose Up
-- Encrypted in-app Secret rows. Rows are immutable; a changed value is a new row. An owner marks
-- a subscriber's personal value, a missing owner an administrative one.
CREATE TABLE configuration_secrets (
    id uuid PRIMARY KEY,
    instance_id uuid NOT NULL REFERENCES processing_instances(id) ON DELETE CASCADE,
    owner_id uuid REFERENCES identity_accounts(id),
    key_id varchar(64) NOT NULL,
    nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
    ciphertext bytea NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE INDEX configuration_secrets_owner ON configuration_secrets (owner_id) WHERE owner_id IS NOT NULL;
CREATE INDEX configuration_secrets_instance ON configuration_secrets (instance_id);

-- One subscriber's user-scoped values for one Delivery instance. A deletion keeps the row with a
-- null configuration (a tombstone) so that the revision never resets.
CREATE TABLE plugin_personal_configurations (
    instance_id uuid NOT NULL REFERENCES processing_instances(id) ON DELETE CASCADE,
    owner_id uuid NOT NULL REFERENCES identity_accounts(id),
    revision bigint NOT NULL CHECK (revision > 0),
    configuration jsonb CHECK (configuration IS NULL OR jsonb_typeof(configuration) = 'object'),
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (instance_id, owner_id)
);
CREATE INDEX plugin_personal_configurations_owner ON plugin_personal_configurations (owner_id);

-- +goose Down
DROP TABLE plugin_personal_configurations;
DROP TABLE configuration_secrets;
