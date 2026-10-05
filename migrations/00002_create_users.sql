-- +goose Up
CREATE TABLE biznes.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text COLLATE "C" NOT NULL,
    password_hash text COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT users_email_unique UNIQUE (email),
    -- ponytail: ASCII email identities; add internationalized addresses with an explicit normalization policy.
    CONSTRAINT users_email_canonical CHECK (
        octet_length(email) BETWEEN 3 AND 254
        AND email = lower(email)
        AND email ~ '^[!-~]+$'
        AND email ~ '^[^@]+@[^@]+$'
    ),
    CONSTRAINT users_password_hash_valid CHECK (
        octet_length(password_hash) BETWEEN 1 AND 1024
        AND password_hash ~ '^[!-~]+$'
    )
);

-- +goose Down
LOCK TABLE biznes.users IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.users) THEN
        RAISE EXCEPTION 'users migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.users RESTRICT;
