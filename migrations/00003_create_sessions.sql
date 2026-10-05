-- +goose Up
CREATE TABLE biznes.sessions (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at timestamptz NOT NULL,
    CONSTRAINT sessions_token_hash_valid CHECK (octet_length(token_hash) = 32),
    CONSTRAINT sessions_times_valid CHECK (
        expires_at > created_at AND last_seen_at >= created_at AND last_seen_at < expires_at
    )
);

-- Expiration is enforced at lookup; this index supports explicit expired-row maintenance.
CREATE INDEX sessions_expires_at_idx ON biznes.sessions (expires_at);

-- +goose Down
LOCK TABLE biznes.sessions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.sessions) THEN
        RAISE EXCEPTION 'sessions migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.sessions RESTRICT;
