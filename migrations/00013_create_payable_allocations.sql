-- +goose Up
ALTER TABLE biznes.payables ADD CONSTRAINT payables_currency_reference_unique
    UNIQUE (organization_id, id, currency);

CREATE TABLE biznes.payable_allocations (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL,
    payable_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    transaction_kind text COLLATE "C" NOT NULL DEFAULT 'expense',
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT payable_allocations_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT payable_allocations_payable_fk FOREIGN KEY (organization_id, payable_id, currency)
        REFERENCES biznes.payables (organization_id, id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT payable_allocations_transaction_fk FOREIGN KEY (organization_id, transaction_id, transaction_kind, currency)
        REFERENCES biznes.transactions (organization_id, id, kind, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT payable_allocations_amount_valid CHECK (amount > 0),
    CONSTRAINT payable_allocations_currency_valid CHECK (currency = 'IRR'),
    CONSTRAINT payable_allocations_kind_valid CHECK (transaction_kind = 'expense')
);

CREATE INDEX payable_allocations_payable_created_id_idx
    ON biznes.payable_allocations (organization_id, payable_id, created_at, id);
CREATE INDEX payable_allocations_transaction_idx ON biznes.payable_allocations (organization_id, transaction_id);
CREATE INDEX payable_allocations_created_by_idx ON biznes.payable_allocations (created_by);

-- +goose Down
LOCK TABLE biznes.payable_allocations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.payable_allocations) THEN
        RAISE EXCEPTION 'payable allocations migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.payable_allocations RESTRICT;
ALTER TABLE biznes.payables DROP CONSTRAINT payables_currency_reference_unique RESTRICT;
