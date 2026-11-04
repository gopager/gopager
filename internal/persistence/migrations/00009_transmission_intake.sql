-- +goose Up
CREATE TABLE networks (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    name varchar(256) NOT NULL CHECK (name <> ''),
    protocol varchar NOT NULL CHECK (protocol IN ('pocsag', 'flex', 'zvei', 'eas')),
    deleted boolean NOT NULL DEFAULT false
);
CREATE TABLE gateways (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    state varchar NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'degraded', 'stopped', 'contact-lost')),
    last_contact_at timestamptz,
    last_heartbeat_at timestamptz,
    last_transmission_at timestamptz,
    last_report jsonb
);
CREATE TABLE gateway_networks (
    id uuid PRIMARY KEY,
    gateway_id uuid NOT NULL REFERENCES gateways(id),
    gateway_network_identifier varchar(128) NOT NULL CHECK (gateway_network_identifier <> ''),
    network_id uuid NOT NULL REFERENCES networks(id),
    is_default boolean NOT NULL DEFAULT false,
    UNIQUE (gateway_id, gateway_network_identifier),
    UNIQUE (gateway_id, network_id)
);
CREATE INDEX gateway_networks_network_id ON gateway_networks (network_id, gateway_id);
-- +goose StatementBegin
CREATE FUNCTION check_gateway_network_defaults() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    affected_gateway uuid;
BEGIN
    affected_gateway := COALESCE(NEW.gateway_id, OLD.gateway_id);
    IF EXISTS (
        SELECT 1
        FROM gateway_networks assignments
        JOIN networks ON networks.id = assignments.network_id
        WHERE assignments.gateway_id = affected_gateway
        GROUP BY networks.protocol
        HAVING count(*) FILTER (WHERE assignments.is_default) <> 1
    ) THEN
        RAISE EXCEPTION 'Gateway must have exactly one default Network per assigned Protocol'
            USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER gateway_network_defaults
    AFTER INSERT OR UPDATE OR DELETE ON gateway_networks
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_gateway_network_defaults();
CREATE TABLE gateway_credentials (
    id uuid PRIMARY KEY,
    gateway_id uuid NOT NULL REFERENCES gateways(id),
    secret_hash bytea NOT NULL UNIQUE CHECK (octet_length(secret_hash) = 32),
    pending boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz
);
CREATE UNIQUE INDEX gateway_credentials_one_current
    ON gateway_credentials (gateway_id) WHERE revoked_at IS NULL AND pending = false;
CREATE UNIQUE INDEX gateway_credentials_one_pending
    ON gateway_credentials (gateway_id) WHERE revoked_at IS NULL AND pending = true;
CREATE TABLE grouping_control (
    id bigint PRIMARY KEY CHECK (id = 1),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    window_seconds bigint NOT NULL CHECK (window_seconds BETWEEN 0 AND 9007199254740991)
);
INSERT INTO grouping_control (id, window_seconds) VALUES (1, 30);
CREATE TABLE messages (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    identity_form varchar NOT NULL CHECK (identity_form <> ''),
    identity_subtype varchar NOT NULL DEFAULT '',
    identity_value bytea NOT NULL,
    first_observed_at timestamptz NOT NULL,
    last_observed_at timestamptz NOT NULL,
    publication_status varchar NOT NULL DEFAULT 'pending'
        CHECK (publication_status IN ('pending', 'ready', 'failed'))
);
CREATE INDEX messages_identity ON messages (identity_form, identity_subtype, identity_value);
CREATE TABLE transmissions (
    id uuid PRIMARY KEY,
    gateway_id uuid NOT NULL REFERENCES gateways(id),
    network_id uuid NOT NULL REFERENCES networks(id),
    submission_id varchar(128) NOT NULL CHECK (submission_id <> ''),
    submission_fingerprint bytea NOT NULL CHECK (octet_length(submission_fingerprint) = 32),
    network_resolution varchar NOT NULL CHECK (network_resolution IN ('declared', 'inferred-single', 'inferred-address', 'default')),
    protocol varchar NOT NULL CHECK (protocol IN ('pocsag', 'flex', 'zvei', 'eas')),
    payload json NOT NULL,
    targets json NOT NULL,
    observed_at timestamptz NOT NULL,
    observation_offset_seconds integer NOT NULL CHECK (observation_offset_seconds BETWEEN -86400 AND 86400),
    accepted_at timestamptz NOT NULL,
    source_revision bigint NOT NULL CHECK (source_revision > 0),
    incomplete boolean NOT NULL DEFAULT false,
    quality_metrics json,
    message_id uuid NOT NULL REFERENCES messages(id),
    processing_status varchar NOT NULL DEFAULT 'pending'
        CHECK (processing_status IN ('pending', 'completed', 'failed')),
    publication_status varchar NOT NULL DEFAULT 'pending'
        CHECK (publication_status IN ('pending', 'ready', 'failed')),
    UNIQUE (gateway_id, submission_id)
);
CREATE INDEX transmissions_protocol_observed ON transmissions (protocol, observed_at, id);
CREATE INDEX transmissions_message_observed ON transmissions (message_id, observed_at, id);
CREATE TABLE transmission_targets (
    transmission_id uuid NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    original json NOT NULL,
    resolution_status varchar NOT NULL CHECK (resolution_status IN ('unresolved', 'unassigned', 'assigned', 'pending', 'failed')),
    unit_id uuid REFERENCES units(id),
    matched_rule_id uuid,
    assignment_revision bigint NOT NULL CHECK (assignment_revision > 0),
    PRIMARY KEY (transmission_id, position)
);
CREATE INDEX transmission_targets_unit ON transmission_targets (unit_id, transmission_id);
CREATE TABLE message_contributions (
    id uuid PRIMARY KEY,
    message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    unit_id uuid REFERENCES units(id),
    contribution_time timestamptz NOT NULL,
    age_anchor timestamptz NOT NULL,
    ready boolean NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX message_contributions_message_unit
    ON message_contributions (message_id, unit_id) WHERE unit_id IS NOT NULL;
CREATE UNIQUE INDEX message_contributions_message_targetless
    ON message_contributions (message_id) WHERE unit_id IS NULL;
CREATE INDEX message_contributions_unit_age ON message_contributions (unit_id, age_anchor);
CREATE TABLE contribution_supports (
    id uuid PRIMARY KEY,
    contribution_id uuid NOT NULL REFERENCES message_contributions(id) ON DELETE CASCADE,
    transmission_id uuid NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
    target_position integer CHECK (target_position >= 0),
    observed_at timestamptz NOT NULL,
    accepted_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX contribution_support_original_target
    ON contribution_supports (transmission_id, target_position) WHERE target_position IS NOT NULL;
CREATE UNIQUE INDEX contribution_support_targetless_transmission
    ON contribution_supports (transmission_id) WHERE target_position IS NULL;
CREATE INDEX contribution_supports_contribution ON contribution_supports (contribution_id);

-- +goose Down
DROP TABLE contribution_supports;
DROP TABLE message_contributions;
DROP TABLE transmission_targets;
DROP TABLE transmissions;
DROP TABLE messages;
DROP TABLE grouping_control;
DROP TABLE gateway_credentials;
DROP TABLE gateway_networks;
DROP TABLE gateways;
DROP TABLE networks;
DROP FUNCTION check_gateway_network_defaults();
