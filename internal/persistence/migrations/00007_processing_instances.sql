-- +goose Up
CREATE TABLE processing_instances (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL CHECK (revision > 0)
);
CREATE TABLE processing_instance_versions (
    id uuid PRIMARY KEY,
    instance_id uuid NOT NULL REFERENCES processing_instances(id),
    revision bigint NOT NULL CHECK (revision > 0),
    configuration jsonb NOT NULL CHECK (
        jsonb_typeof(configuration) = 'object'
        AND configuration->>'id' = instance_id::text
        AND configuration->>'revision' = revision::text
    ),
    UNIQUE (instance_id, revision)
);
ALTER TABLE processing_instances ADD CONSTRAINT processing_instance_current_version
    FOREIGN KEY (id, revision) REFERENCES processing_instance_versions(instance_id, revision)
    DEFERRABLE INITIALLY DEFERRED;

-- +goose Down
ALTER TABLE processing_instances DROP CONSTRAINT processing_instance_current_version;
DROP TABLE processing_instance_versions;
DROP TABLE processing_instances;
