-- +goose Up
ALTER TABLE biznes.receivables ADD CONSTRAINT receivables_currency_reference_unique
    UNIQUE (organization_id, id, currency);
ALTER TABLE biznes.transactions ADD CONSTRAINT transactions_collection_reference_unique
    UNIQUE (organization_id, id, kind, currency);

CREATE TABLE biznes.receivable_allocations (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL,
    receivable_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    transaction_kind text COLLATE "C" NOT NULL DEFAULT 'income',
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT receivable_allocations_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT receivable_allocations_receivable_fk FOREIGN KEY (organization_id, receivable_id, currency)
        REFERENCES biznes.receivables (organization_id, id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT receivable_allocations_transaction_fk FOREIGN KEY (organization_id, transaction_id, transaction_kind, currency)
        REFERENCES biznes.transactions (organization_id, id, kind, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT receivable_allocations_amount_valid CHECK (amount > 0),
    CONSTRAINT receivable_allocations_currency_valid CHECK (currency = 'IRR'),
    CONSTRAINT receivable_allocations_kind_valid CHECK (transaction_kind = 'income')
);

CREATE INDEX receivable_allocations_receivable_created_id_idx
    ON biznes.receivable_allocations (organization_id, receivable_id, created_at, id);
CREATE INDEX receivable_allocations_transaction_idx ON biznes.receivable_allocations (organization_id, transaction_id);
CREATE INDEX receivable_allocations_created_by_idx ON biznes.receivable_allocations (created_by);

-- +goose Down
LOCK TABLE biznes.receivable_allocations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.receivable_allocations) THEN
        RAISE EXCEPTION 'receivable allocations migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.receivable_allocations RESTRICT;
ALTER TABLE biznes.transactions DROP CONSTRAINT transactions_collection_reference_unique RESTRICT;
ALTER TABLE biznes.receivables DROP CONSTRAINT receivables_currency_reference_unique RESTRICT;
