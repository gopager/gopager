-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM identity_grants WHERE field_names <> '[]'::jsonb) THEN
        RAISE EXCEPTION 'INCOMPATIBLE_VERSION: scoped development field permissions require explicit review';
    END IF;
END;
$$;
-- +goose StatementEnd

-- Retain the unpublished legacy column without using it for authorization.
ALTER TABLE identity_grants ALTER COLUMN field_names SET DEFAULT '[]'::jsonb;
CREATE TABLE identity_field_permissions (
    id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    subject_id uuid NOT NULL,
    subject_kind varchar NOT NULL CHECK (subject_kind IN ('principal', 'userGroup')),
    field_names jsonb NOT NULL CHECK (jsonb_typeof(field_names) = 'array' AND jsonb_array_length(field_names) > 0),
    delegable boolean NOT NULL DEFAULT false
);
CREATE INDEX fieldpermission_subject_id_subject_kind ON identity_field_permissions (subject_id, subject_kind);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM identity_field_permissions) THEN
        RAISE EXCEPTION 'INCOMPATIBLE_VERSION: global field permissions cannot become scoped grants';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE identity_field_permissions;
ALTER TABLE identity_grants ALTER COLUMN field_names DROP DEFAULT;
