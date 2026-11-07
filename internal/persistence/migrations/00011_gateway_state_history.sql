-- +goose Up
-- One row per Gateway revision that changed the Gateway state; the unique key allows at
-- most one transition per revision. Written in the transaction of the state change and its
-- gateway.state-changed fact.
CREATE TABLE gateway_state_history (
    id uuid PRIMARY KEY,
    gateway_id uuid NOT NULL REFERENCES gateways(id),
    revision bigint NOT NULL CHECK (revision > 0),
    old_state varchar NOT NULL
        CHECK (old_state IN ('never-seen', 'active', 'degraded', 'stopped', 'contact-lost')),
    new_state varchar NOT NULL
        CHECK (new_state IN ('active', 'degraded', 'stopped', 'contact-lost') AND new_state <> old_state),
    cause varchar NOT NULL CHECK (cause IN ('report', 'submission', 'timeout')),
    occurred_at timestamptz NOT NULL,
    contact_at timestamptz NOT NULL,
    reason_code varchar(64),
    detail varchar(512),
    UNIQUE (gateway_id, revision)
);

-- +goose Down
DROP TABLE gateway_state_history;
