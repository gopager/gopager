-- +goose Up
-- domain_events is the sequenced fact log. seq orders consumption; tx_id lets readers skip
-- rows whose transaction may still be in flight (see eventstore.Store.ReadAfter), because
-- identity values are assigned at insert time and become visible in commit order, not seq order.
-- The UNIQUE constraint on seq provides the (seq) index.

-- IMMUTABLE is asserted deliberately: the text-to-timestamptz cast is only STABLE because it
-- depends on DateStyle/TimeZone, but the envelope always carries an RFC 3339 timestamp with an
-- explicit offset, which parses identically in every session. PostgreSQL requires an
-- immutable expression for generated columns.
-- +goose StatementBegin
CREATE FUNCTION domain_event_occurred_at(envelope jsonb) RETURNS timestamptz
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT (envelope->>'occurredAt')::timestamptz $$;
-- +goose StatementEnd

CREATE TABLE domain_events (
    id uuid PRIMARY KEY,
    seq bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    tx_id xid8 NOT NULL DEFAULT pg_current_xact_id(),
    envelope jsonb NOT NULL,
    type text GENERATED ALWAYS AS (envelope->>'type') STORED,
    occurred_at timestamptz GENERATED ALWAYS AS (domain_event_occurred_at(envelope)) STORED,
    CHECK (envelope->>'domainEventId' = id::text)
);
CREATE INDEX domain_events_type_seq_idx ON domain_events (type, seq);

-- One durable read position per named consumer of domain_events.
CREATE TABLE consumer_offsets (
    consumer varchar PRIMARY KEY,
    last_seq bigint NOT NULL CHECK (last_seq >= 0),
    updated_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE consumer_offsets;
DROP TABLE domain_events;
DROP FUNCTION domain_event_occurred_at(jsonb);
