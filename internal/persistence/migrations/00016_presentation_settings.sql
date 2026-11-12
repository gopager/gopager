-- +goose Up
-- The single row of installation-wide presentation settings: the title and notice banner every
-- client shows, and the Layout used when no narrower layer chooses one.
CREATE TABLE installation_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    revision bigint NOT NULL CHECK (revision > 0),
    title varchar(128) NOT NULL CHECK (char_length(title) BETWEEN 1 AND 128),
    banner varchar(2048),
    severity varchar CHECK (severity IN ('info', 'warning', 'critical')),
    default_layout_id uuid REFERENCES layout_definitions(id),
    updated_at timestamptz NOT NULL,
    CHECK ((banner IS NULL) = (severity IS NULL))
);
INSERT INTO installation_settings (id, revision, title, updated_at) VALUES (1, 1, 'GoPager', now());

-- At most one Layout per Unit or Organization and output kind. The scope is not a foreign key
-- because it names rows of two tables; readers skip assignments of deleted scopes.
CREATE TABLE layout_assignments (
    scope_kind varchar NOT NULL CHECK (scope_kind IN ('organization', 'unit')),
    scope_id uuid NOT NULL,
    output_kind varchar NOT NULL CHECK (output_kind IN ('text', 'markdown', 'json')),
    layout_id uuid NOT NULL REFERENCES layout_definitions(id),
    revision bigint NOT NULL CHECK (revision > 0),
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (scope_kind, scope_id, output_kind)
);
CREATE INDEX layout_assignments_layout ON layout_assignments (layout_id);

-- Units and Organizations are deleted by a flag, so their assignments go with that flag.
-- +goose StatementBegin
CREATE FUNCTION remove_layout_assignments_of_deleted_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    DELETE FROM layout_assignments WHERE scope_kind = TG_ARGV[0] AND scope_id = NEW.id;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER units_remove_layout_assignments AFTER UPDATE OF deleted ON units
    FOR EACH ROW WHEN (NEW.deleted AND NOT OLD.deleted)
    EXECUTE FUNCTION remove_layout_assignments_of_deleted_scope('unit');
CREATE TRIGGER organizations_remove_layout_assignments AFTER UPDATE OF deleted ON organizations
    FOR EACH ROW WHEN (NEW.deleted AND NOT OLD.deleted)
    EXECUTE FUNCTION remove_layout_assignments_of_deleted_scope('organization');

-- +goose Down
DROP TRIGGER organizations_remove_layout_assignments ON organizations;
DROP TRIGGER units_remove_layout_assignments ON units;
DROP FUNCTION remove_layout_assignments_of_deleted_scope();
DROP TABLE layout_assignments;
DROP TABLE installation_settings;
