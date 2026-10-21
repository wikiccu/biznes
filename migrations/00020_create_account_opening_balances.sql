-- +goose Up
CREATE TABLE biznes.account_opening_balances (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    effective_at timestamptz NOT NULL,
    reason text NOT NULL,
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT account_opening_balances_account_unique UNIQUE (organization_id, account_id),
    CONSTRAINT account_opening_balances_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT account_opening_balances_account_fk FOREIGN KEY (organization_id, account_id, currency)
        REFERENCES biznes.financial_accounts (organization_id, id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT account_opening_balances_currency_valid CHECK (currency = 'IRR'),
    CONSTRAINT account_opening_balances_effective_at_valid CHECK (effective_at >= TIMESTAMPTZ '0001-01-01 00:00:00+00'
        AND effective_at < TIMESTAMPTZ '10000-01-01 00:00:00+00'),
    CONSTRAINT account_opening_balances_reason_valid CHECK (char_length(reason) BETWEEN 1 AND 2000 AND reason !~ '[[:cntrl:]]')
);

CREATE INDEX account_opening_balances_created_by_idx ON biznes.account_opening_balances (created_by);

-- +goose Down
LOCK TABLE biznes.account_opening_balances IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.account_opening_balances) THEN
        RAISE EXCEPTION 'account opening balances migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.account_opening_balances RESTRICT;
