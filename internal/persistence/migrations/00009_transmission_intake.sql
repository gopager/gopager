-- +goose Up
CREATE TABLE networks (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    name varchar(256) NOT NULL CHECK (name <> ''),
    protocol varchar NOT NULL CHECK (protocol IN ('pocsag', 'flex', 'zvei', 'eas')),
    agency_id uuid NOT NULL REFERENCES agencies(id),
    deleted boolean NOT NULL DEFAULT false
);
CREATE INDEX networks_agency_id ON networks (agency_id);
CREATE TABLE gateways (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    -- The management API always sets a non-blank name; the empty default keeps directly
    -- inserted rows valid.
    name varchar(256) NOT NULL DEFAULT '',
    -- A disabled or deleted Gateway authenticates nothing and is not evaluated for contact loss.
    enabled boolean NOT NULL DEFAULT true,
    deleted boolean NOT NULL DEFAULT false,
    state varchar NOT NULL DEFAULT 'never-seen'
        CHECK (state IN ('never-seen', 'active', 'degraded', 'stopped', 'contact-lost')),
    last_contact_at timestamptz,
    last_heartbeat_at timestamptz,
    last_transmission_at timestamptz,
    last_report jsonb,
    -- Set by retention to its cutoff when it deletes older gateway_state_history rows.
    history_removed_before timestamptz
);
-- Serves the contact-timeout scan over evaluable Gateways.
CREATE INDEX gateways_contact_scan ON gateways (last_contact_at)
    WHERE enabled AND NOT deleted AND state IN ('active', 'degraded');
CREATE TABLE gateway_networks (
    id uuid PRIMARY KEY,
    gateway_id uuid NOT NULL REFERENCES gateways(id),
    gateway_network_identifier varchar(128) NOT NULL CHECK (gateway_network_identifier <> ''),
    network_id uuid NOT NULL REFERENCES networks(id),
    -- Resolution order among the Gateway's Networks: a lower number is a higher priority.
    priority integer NOT NULL CHECK (priority >= 0),
    UNIQUE (gateway_id, gateway_network_identifier),
    UNIQUE (gateway_id, network_id),
    UNIQUE (gateway_id, priority)
);
CREATE INDEX gateway_networks_network_id ON gateway_networks (network_id, gateway_id);
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
    last_observed_at timestamptz NOT NULL
);
CREATE INDEX messages_identity ON messages (identity_form, identity_subtype, identity_value, last_observed_at);
-- The feed pages by (first_observed_at, id) descending; the index serves that keyset order directly.
CREATE INDEX messages_first_observed ON messages (first_observed_at DESC, id DESC);
-- Published Processing results belong to the Message of their Transmission; a merge moves them.
ALTER TABLE processing_results ADD CONSTRAINT processing_results_message
    FOREIGN KEY (message_id) REFERENCES messages(id);
CREATE TABLE transmissions (
    id uuid PRIMARY KEY,
    gateway_id uuid NOT NULL REFERENCES gateways(id),
    network_id uuid NOT NULL REFERENCES networks(id),
    submission_id varchar(128) NOT NULL CHECK (submission_id <> ''),
    submission_fingerprint bytea NOT NULL CHECK (octet_length(submission_fingerprint) = 32),
    network_resolution varchar NOT NULL CHECK (network_resolution IN ('declared', 'inferred-single', 'inferred-address', 'address-priority', 'priority')),
    protocol varchar NOT NULL CHECK (protocol IN ('pocsag', 'flex', 'zvei', 'eas')),
    payload json NOT NULL,
    observed_at timestamptz NOT NULL,
    observation_offset_seconds integer NOT NULL CHECK (observation_offset_seconds BETWEEN -86400 AND 86400),
    accepted_at timestamptz NOT NULL,
    source_revision bigint NOT NULL CHECK (source_revision > 0),
    incomplete boolean NOT NULL DEFAULT false,
    quality_metrics json,
    message_id uuid NOT NULL REFERENCES messages(id),
    processing_status varchar NOT NULL DEFAULT 'pending'
        CHECK (processing_status IN ('none', 'pending', 'completed', 'failed')),
    -- The declared Gateway Network identifier (NULL when none was declared), the assignment
    -- revision at which the Network was last resolved and whether the current Gateway mapping
    -- could not resolve it: the inputs and state of a Gateway mapping re-resolution.
    gateway_network_identifier varchar(128) CHECK (gateway_network_identifier <> ''),
    network_revision bigint NOT NULL DEFAULT 1 CHECK (network_revision > 0),
    network_unresolved boolean NOT NULL DEFAULT false,
    UNIQUE (gateway_id, submission_id)
);
CREATE INDEX transmissions_protocol_observed ON transmissions (protocol, observed_at, id);
CREATE INDEX transmissions_gateway_accepted ON transmissions (gateway_id, protocol, accepted_at, id);
CREATE INDEX transmissions_message_observed ON transmissions (message_id, observed_at, id);
-- Each target carries its Network, Protocol, observation time and typed address so that a
-- selector-scoped correction and the Unit history are index lookups; original stays the evidence.
CREATE TABLE transmission_targets (
    transmission_id uuid NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    original json NOT NULL,
    network_id uuid NOT NULL REFERENCES networks(id),
    protocol varchar NOT NULL CHECK (protocol IN ('pocsag', 'flex', 'zvei', 'eas')),
    observed_at timestamptz NOT NULL,
    accepted_at timestamptz NOT NULL,
    address_number bigint,
    address_text varchar,
    function smallint CHECK (function BETWEEN 0 AND 255),
    resolution_status varchar NOT NULL CHECK (resolution_status IN ('unresolved', 'unassigned', 'assigned', 'pending', 'failed')),
    unit_id uuid REFERENCES units(id),
    matched_rule_id uuid,
    assignment_revision bigint NOT NULL CHECK (assignment_revision > 0),
    PRIMARY KEY (transmission_id, position),
    CHECK ((address_number IS NULL) <> (address_text IS NULL))
);
CREATE INDEX transmission_targets_numeric_address ON transmission_targets (network_id, protocol, address_number, observed_at)
    WHERE address_number IS NOT NULL;
CREATE INDEX transmission_targets_text_address ON transmission_targets (network_id, protocol, address_text, observed_at)
    WHERE address_text IS NOT NULL;
CREATE INDEX transmission_targets_unit ON transmission_targets (unit_id, observed_at);
-- message_units is the rebuildable cache of which Units a Message alerts: one row per Message and
-- Unit (with the Unit's Agency) and one Unit-less row per Message and Network Agency for
-- targetless Transmissions. Intake upserts it, a correction rebuilds it per affected Message.
CREATE TABLE message_units (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    unit_id uuid REFERENCES units(id),
    agency_id uuid NOT NULL REFERENCES agencies(id),
    first_observed_at timestamptz NOT NULL,
    age_anchor timestamptz NOT NULL,
    correction_status varchar NOT NULL DEFAULT 'current' CHECK (correction_status IN ('current', 'correcting'))
);
CREATE UNIQUE INDEX message_units_message_unit ON message_units (message_id, unit_id) WHERE unit_id IS NOT NULL;
CREATE UNIQUE INDEX message_units_message_targetless ON message_units (message_id, agency_id) WHERE unit_id IS NULL;
-- The feed's eligibility check probes message_units by Message; the partial unique indexes above cannot serve it.
CREATE INDEX message_units_message ON message_units (message_id);
CREATE INDEX message_units_unit_age ON message_units (unit_id, age_anchor);
CREATE INDEX message_units_agency_time ON message_units (agency_id, first_observed_at);

-- +goose Down
ALTER TABLE processing_results DROP CONSTRAINT processing_results_message;
DROP TABLE message_units;
DROP TABLE transmission_targets;
DROP TABLE transmissions;
DROP TABLE messages;
DROP TABLE grouping_control;
DROP TABLE gateway_credentials;
DROP TABLE gateway_networks;
DROP TABLE gateways;
DROP TABLE networks;
DROP FUNCTION check_gateway_network_defaults();
