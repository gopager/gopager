-- +goose Up
-- Display preferences of one interactive account; an account without a row uses the defaults.
CREATE TABLE account_preferences (
    account_id uuid PRIMARY KEY REFERENCES identity_accounts(id) ON DELETE CASCADE,
    revision bigint NOT NULL CHECK (revision > 0),
    locale varchar(35) NOT NULL CHECK (locale <> ''),
    time_zone varchar NOT NULL CHECK (time_zone <> ''),
    time_gap_minutes integer NOT NULL CHECK (time_gap_minutes BETWEEN 0 AND 1440),
    updated_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE account_preferences;
