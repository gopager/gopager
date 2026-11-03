-- +goose Up
CREATE TABLE domain_events (
    id uuid PRIMARY KEY,
    envelope jsonb NOT NULL,
    CHECK (envelope->>'domainEventId' = id::text)
);

-- +goose Down
DROP TABLE domain_events;
