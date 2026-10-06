-- +goose Up
CREATE TABLE biznes.contacts (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL,
    kind text COLLATE "C" NOT NULL,
    email text NOT NULL DEFAULT '',
    phone text NOT NULL DEFAULT '',
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT contacts_name_valid CHECK (char_length(name) BETWEEN 1 AND 120 AND name !~ '[[:cntrl:]]'),
    CONSTRAINT contacts_kind_valid CHECK (kind IN ('customer', 'supplier', 'both')),
    CONSTRAINT contacts_email_valid CHECK (octet_length(email) <= 254 AND email !~ '[[:cntrl:]]'),
    CONSTRAINT contacts_phone_valid CHECK (char_length(phone) <= 64 AND phone !~ '[[:cntrl:]]'),
    CONSTRAINT contacts_notes_valid CHECK (
        char_length(notes) <= 2000 AND translate(notes, E'\n\r\t', '') !~ '[[:cntrl:]]'
    )
);

CREATE INDEX contacts_organization_created_id_idx ON biznes.contacts (organization_id, created_at, id);

-- +goose Down
LOCK TABLE biznes.contacts IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.contacts) THEN
        RAISE EXCEPTION 'contacts migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.contacts RESTRICT;
