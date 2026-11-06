-- +goose Up
CREATE TABLE target_suppressions (
    id uuid PRIMARY KEY,
    transmission_id uuid NOT NULL,
    target_position integer NOT NULL CHECK (target_position >= 0),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL,
    created_by uuid NOT NULL,
    removed_at timestamptz,
    removed_by uuid,
    FOREIGN KEY (transmission_id, target_position)
        REFERENCES transmission_targets (transmission_id, position)
);
CREATE UNIQUE INDEX target_suppressions_active_target
    ON target_suppressions (transmission_id, target_position) WHERE removed_at IS NULL;

-- +goose Down
DROP TABLE target_suppressions;
