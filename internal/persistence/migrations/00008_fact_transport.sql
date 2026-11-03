-- +goose Up
-- Watermill SQL v4 DefaultPostgreSQLSchema/DefaultPostgreSQLOffsetsAdapter layout.
-- Only references live here; domain_events retains the classified canonical envelopes.
CREATE TABLE watermill_domain_facts (
    "offset" BIGSERIAL,
    uuid VARCHAR(36) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    payload JSON DEFAULT NULL,
    metadata JSON DEFAULT NULL,
    transaction_id xid8 NOT NULL,
    PRIMARY KEY (transaction_id, "offset")
);
CREATE TABLE watermill_offsets_domain_facts (
    consumer_group VARCHAR(255) NOT NULL,
    offset_acked BIGINT,
    last_processed_transaction_id xid8 NOT NULL,
    PRIMARY KEY (consumer_group)
);

-- +goose Down
DROP TABLE watermill_offsets_domain_facts;
DROP TABLE watermill_domain_facts;
