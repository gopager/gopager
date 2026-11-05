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

CREATE TABLE correction_dirty_transmissions (
    scope_id uuid NOT NULL REFERENCES assignment_pending_scopes(id) ON DELETE CASCADE,
    transmission_id uuid NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
    accepted_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL,
    UNIQUE (scope_id, accepted_at, transmission_id)
);
CREATE INDEX correction_dirty_transmissions_transmission
    ON correction_dirty_transmissions (transmission_id);

-- +goose Down
DROP TABLE correction_dirty_transmissions;
DROP TABLE target_suppressions;
