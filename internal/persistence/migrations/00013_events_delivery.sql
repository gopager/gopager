-- +goose Up
-- A layout arranges permitted fields for one output kind; every save appends an immutable version
-- whose number equals the layout's revision.
CREATE TABLE layout_definitions (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL CHECK (revision > 0),
    name varchar(256) NOT NULL CHECK (name <> ''),
    output_kind varchar NOT NULL CHECK (output_kind IN ('text', 'markdown', 'json')),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE TABLE layout_definition_versions (
    layout_id uuid NOT NULL REFERENCES layout_definitions(id) ON DELETE CASCADE,
    version bigint NOT NULL CHECK (version > 0),
    definition jsonb NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (layout_id, version)
);

-- A Subscription selects Domain event types for one subscriber: the owning User of a personal
-- Subscription or the technical account of a global one.
CREATE TABLE subscriptions (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL CHECK (revision > 0),
    kind varchar NOT NULL CHECK (kind IN ('personal', 'global')),
    subscriber_id uuid NOT NULL,
    created_by uuid NOT NULL,
    state varchar NOT NULL CHECK (state IN ('active', 'paused')),
    event_types text[] NOT NULL CHECK (cardinality(event_types) BETWEEN 1 AND 16),
    filter jsonb CHECK (filter IS NULL OR jsonb_typeof(filter) = 'object'),
    layout_id uuid REFERENCES layout_definitions(id),
    split_by_unit boolean NOT NULL,
    max_age_seconds integer CHECK (max_age_seconds BETWEEN 1 AND 86400),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX subscriptions_subscriber ON subscriptions (subscriber_id);
CREATE INDEX subscriptions_active_types ON subscriptions USING gin (event_types) WHERE state = 'active';
CREATE INDEX subscriptions_layout ON subscriptions (layout_id) WHERE layout_id IS NOT NULL;

-- Each target is a configured instance with the Delivery capability; inputs are typed constants.
CREATE TABLE subscription_targets (
    subscription_id uuid NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 7),
    instance_id uuid NOT NULL REFERENCES processing_instances(id),
    capability_kind varchar NOT NULL CHECK (capability_kind = 'delivery'),
    instance_revision bigint NOT NULL CHECK (instance_revision > 0),
    inputs jsonb NOT NULL CHECK (jsonb_typeof(inputs) = 'object'),
    PRIMARY KEY (subscription_id, position),
    UNIQUE (subscription_id, instance_id)
);
CREATE INDEX subscription_targets_instance ON subscription_targets (instance_id);

-- Unit-specific values of a target instance; only keys the instance's descriptor declares apply.
CREATE TABLE integration_unit_parameters (
    instance_id uuid NOT NULL REFERENCES processing_instances(id) ON DELETE CASCADE,
    unit_id uuid NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    key varchar(128) NOT NULL CHECK (key <> ''),
    value jsonb NOT NULL CHECK (jsonb_typeof(value) = 'object'),
    PRIMARY KEY (instance_id, unit_id, key)
);

-- A Delivery is one job per Subscription, Domain event, target instance and split. The input is
-- the bounded materialized Payload and is removed when the Delivery ends. Subscription and
-- subscriber carry no foreign key, so history outlives a deleted Subscription.
CREATE TABLE deliveries (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL CHECK (revision > 0),
    origin varchar NOT NULL CHECK (origin IN ('automatic', 'redelivery')),
    redelivery_of uuid REFERENCES deliveries(id) ON DELETE SET NULL,
    subscription_id uuid NOT NULL,
    subscription_revision bigint NOT NULL CHECK (subscription_revision > 0),
    subscriber_id uuid NOT NULL,
    domain_event_id uuid NOT NULL,
    event_type varchar NOT NULL,
    correlation_id uuid,
    causation_id uuid NOT NULL,
    target_instance_id uuid NOT NULL,
    split_key varchar NOT NULL,
    state varchar NOT NULL CHECK (state IN ('prepared', 'started', 'retry-pending', 'accepted', 'failed',
        'uncertain', 'expired', 'cancelled')),
    outcome_code varchar,
    attempts integer NOT NULL CHECK (attempts >= 0),
    layout_id uuid,
    layout_version bigint NOT NULL CHECK (layout_version >= 0),
    input jsonb,
    context jsonb NOT NULL CHECK (jsonb_typeof(context) = 'object'),
    message_id uuid,
    unit_ids uuid[] NOT NULL,
    agency_ids uuid[] NOT NULL,
    age_anchor timestamptz NOT NULL,
    access_revision bigint NOT NULL CHECK (access_revision > 0),
    claim_token bigint NOT NULL CHECK (claim_token >= 0),
    claim_expires_at timestamptz,
    next_attempt_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    finished_at timestamptz,
    CHECK ((state IN ('accepted', 'failed', 'uncertain', 'expired', 'cancelled')) = (finished_at IS NOT NULL)),
    CHECK (finished_at IS NULL OR input IS NULL),
    CHECK (origin = 'redelivery' OR redelivery_of IS NULL)
);
CREATE UNIQUE INDEX deliveries_identity ON deliveries (subscription_id, domain_event_id, target_instance_id, split_key)
    WHERE origin = 'automatic';
CREATE INDEX deliveries_due ON deliveries (next_attempt_at) WHERE state IN ('prepared', 'retry-pending');
CREATE INDEX deliveries_started ON deliveries (claim_expires_at) WHERE state = 'started';
CREATE INDEX deliveries_subscriber ON deliveries (subscriber_id);
CREATE INDEX deliveries_subscription ON deliveries (subscription_id, created_at);

CREATE TABLE delivery_attempts (
    delivery_id uuid NOT NULL REFERENCES deliveries(id) ON DELETE CASCADE,
    number integer NOT NULL CHECK (number > 0),
    id uuid NOT NULL UNIQUE,
    state varchar NOT NULL CHECK (state IN ('started', 'accepted', 'transient', 'permanent', 'uncertain')),
    outcome_code varchar,
    instance_revision bigint NOT NULL CHECK (instance_revision > 0),
    access_revision bigint NOT NULL CHECK (access_revision > 0),
    receipt jsonb,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    PRIMARY KEY (delivery_id, number),
    CHECK ((state = 'started') = (finished_at IS NULL))
);

-- Fixed one-minute sending admission windows, keyed by a hashed subscriber or target bucket.
CREATE TABLE delivery_admissions (
    bucket bytea NOT NULL CHECK (octet_length(bucket) = 32),
    window_start timestamptz NOT NULL,
    count integer NOT NULL CHECK (count > 0),
    PRIMARY KEY (bucket, window_start)
);
-- Serves the bounded sweep of expired windows of buckets that do not come back.
CREATE INDEX delivery_admissions_window_start ON delivery_admissions (window_start);

-- +goose Down
DROP TABLE delivery_admissions;
DROP TABLE delivery_attempts;
DROP TABLE deliveries;
DROP TABLE integration_unit_parameters;
DROP TABLE subscription_targets;
DROP TABLE subscriptions;
DROP TABLE layout_definition_versions;
DROP TABLE layout_definitions;
