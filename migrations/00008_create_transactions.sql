-- +goose Up
ALTER TABLE biznes.financial_accounts ADD CONSTRAINT financial_accounts_currency_reference_unique UNIQUE (organization_id, id, currency);
ALTER TABLE biznes.transaction_categories ADD CONSTRAINT transaction_categories_kind_reference_unique UNIQUE (organization_id, id, kind);

CREATE TABLE biznes.transactions (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL,
    account_id uuid NOT NULL,
    category_id uuid NOT NULL,
    kind text COLLATE "C" NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    occurred_at timestamptz NOT NULL,
    description text NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT transactions_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT transactions_account_fk FOREIGN KEY (organization_id, account_id, currency)
        REFERENCES biznes.financial_accounts (organization_id, id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT transactions_category_fk FOREIGN KEY (organization_id, category_id, kind)
        REFERENCES biznes.transaction_categories (organization_id, id, kind) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT transactions_kind_valid CHECK (kind IN ('income', 'expense')),
    CONSTRAINT transactions_amount_valid CHECK (amount > 0),
    CONSTRAINT transactions_currency_valid CHECK (currency = 'IRR'),
    CONSTRAINT transactions_occurred_at_valid CHECK (occurred_at >= TIMESTAMPTZ '0001-01-01 00:00:00+00'
        AND occurred_at < TIMESTAMPTZ '10000-01-01 00:00:00+00'),
    CONSTRAINT transactions_description_valid CHECK (char_length(description) <= 2000
        AND translate(description, E'\n\r\t', '') !~ '[[:cntrl:]]')
);

CREATE INDEX transactions_organization_created_id_idx ON biznes.transactions (organization_id, created_at, id);
CREATE INDEX transactions_account_idx ON biznes.transactions (organization_id, account_id);
CREATE INDEX transactions_category_idx ON biznes.transactions (organization_id, category_id);
CREATE INDEX transactions_created_by_idx ON biznes.transactions (created_by);

-- +goose Down
LOCK TABLE biznes.transactions IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.transactions) THEN
        RAISE EXCEPTION 'transactions migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.transactions RESTRICT;
ALTER TABLE biznes.transaction_categories DROP CONSTRAINT transaction_categories_kind_reference_unique RESTRICT;
ALTER TABLE biznes.financial_accounts DROP CONSTRAINT financial_accounts_currency_reference_unique RESTRICT;
