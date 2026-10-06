-- +goose Up
CREATE TABLE biznes.financial_accounts (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text COLLATE "C" NOT NULL,
    kind text COLLATE "C" NOT NULL,
    currency text COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT financial_accounts_name_valid CHECK (char_length(name) BETWEEN 1 AND 120 AND name !~ '[[:cntrl:]]'),
    CONSTRAINT financial_accounts_kind_valid CHECK (kind IN ('cash', 'bank')),
    CONSTRAINT financial_accounts_currency_valid CHECK (currency = 'IRR'),
    CONSTRAINT financial_accounts_name_unique UNIQUE (organization_id, name)
);

CREATE INDEX financial_accounts_organization_created_id_idx ON biznes.financial_accounts (organization_id, created_at, id);

-- +goose Down
LOCK TABLE biznes.financial_accounts IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.financial_accounts) THEN
        RAISE EXCEPTION 'financial accounts migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.financial_accounts RESTRICT;
