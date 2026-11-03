-- +goose Up
CREATE TABLE identity_login_attempts (
    id uuid PRIMARY KEY,
    account_key bytea NOT NULL CHECK (octet_length(account_key) = 32),
    attempted_at timestamptz NOT NULL
);
CREATE INDEX login_attempt_attempted_at ON identity_login_attempts (attempted_at);
CREATE INDEX login_attempt_account_key_attempted_at ON identity_login_attempts (account_key, attempted_at);

-- +goose Down
DROP TABLE identity_login_attempts;
