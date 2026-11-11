-- +goose Up
-- The single control row counts every change of the Attribute-schema catalog, so a cache of the
-- catalog can tell whether it is current.
CREATE TABLE attribute_catalog (
    id smallint PRIMARY KEY CHECK (id = 1),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0)
);
INSERT INTO attribute_catalog (id, revision) VALUES (1, 1);

-- An Attribute schema declares the typed fields of Message or Unit attributes under an
-- immutable key; the fields document is validated by the application.
CREATE TABLE attribute_schemas (
    key varchar(64) PRIMARY KEY,
    revision bigint NOT NULL CHECK (revision > 0),
    label varchar NOT NULL CHECK (label <> ''),
    description text NOT NULL DEFAULT '',
    subject varchar NOT NULL CHECK (subject IN ('message', 'unit')),
    fields jsonb NOT NULL CHECK (jsonb_typeof(fields) = 'object'),
    deprecated boolean NOT NULL DEFAULT false,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

-- A schema belongs to one or more Agencies; they decide who may see and manage it.
CREATE TABLE attribute_schema_agencies (
    schema_key varchar(64) NOT NULL REFERENCES attribute_schemas(key) ON DELETE CASCADE,
    agency_id uuid NOT NULL REFERENCES agencies(id) ON DELETE CASCADE,
    PRIMARY KEY (schema_key, agency_id)
);
CREATE INDEX attribute_schema_agencies_agency ON attribute_schema_agencies (agency_id);

-- +goose Down
DROP TABLE attribute_schema_agencies;
DROP TABLE attribute_schemas;
DROP TABLE attribute_catalog;
