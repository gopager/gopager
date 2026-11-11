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

-- The typed projection of the values of published Processing results: one row per scalar path
-- of a field (a list yields a row per element, an object its scalar descendants) and target
-- position. It holds every published value, not the effective one, because what is effective
-- depends on what the reader may see; the read side uses it only as an indexed prefilter before
-- the exact check. It is written in the publication transaction and removed with the result.
CREATE TABLE message_attribute_values (
    result_id uuid NOT NULL REFERENCES processing_results(id) ON DELETE CASCADE,
    transmission_id uuid NOT NULL,
    schema_key varchar(64) NOT NULL,
    path text NOT NULL,
    target_position integer,
    text_value text,
    integer_value bigint,
    number_value double precision,
    time_value timestamptz,
    boolean_value boolean,
    CHECK (num_nonnulls(text_value, integer_value, number_value, time_value, boolean_value) <= 1)
);
CREATE INDEX message_attribute_values_result ON message_attribute_values (result_id);
CREATE INDEX message_attribute_values_text ON message_attribute_values (schema_key, path, text_value)
    WHERE text_value IS NOT NULL;
CREATE INDEX message_attribute_values_integer ON message_attribute_values (schema_key, path, integer_value)
    WHERE integer_value IS NOT NULL;
CREATE INDEX message_attribute_values_number ON message_attribute_values (schema_key, path, number_value)
    WHERE number_value IS NOT NULL;
CREATE INDEX message_attribute_values_time ON message_attribute_values (schema_key, path, time_value)
    WHERE time_value IS NOT NULL;
CREATE INDEX message_attribute_values_boolean ON message_attribute_values (schema_key, path, boolean_value)
    WHERE boolean_value IS NOT NULL;

-- +goose Down
DROP TABLE message_attribute_values;
DROP TABLE attribute_schema_agencies;
DROP TABLE attribute_schemas;
DROP TABLE attribute_catalog;
