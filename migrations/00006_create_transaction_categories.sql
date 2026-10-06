-- +goose Up
CREATE TABLE biznes.transaction_categories (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text COLLATE "C" NOT NULL,
    kind text COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT transaction_categories_name_valid CHECK (char_length(name) BETWEEN 1 AND 120 AND name !~ '[[:cntrl:]]'),
    CONSTRAINT transaction_categories_kind_valid CHECK (kind IN ('income', 'expense')),
    CONSTRAINT transaction_categories_name_unique UNIQUE (organization_id, kind, name)
);

CREATE INDEX transaction_categories_organization_created_id_idx ON biznes.transaction_categories (organization_id, created_at, id);

-- +goose Down
LOCK TABLE biznes.transaction_categories IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.transaction_categories) THEN
        RAISE EXCEPTION 'transaction categories migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.transaction_categories RESTRICT;
