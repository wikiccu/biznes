-- +goose Up
ALTER TABLE biznes.receivable_instruments ADD CONSTRAINT receivable_instruments_settlement_reference_unique
    UNIQUE (organization_id, id, receivable_id, currency);
ALTER TABLE biznes.receivable_allocations ADD CONSTRAINT receivable_allocations_settlement_reference_unique
    UNIQUE (organization_id, id, receivable_id, currency);

CREATE TABLE biznes.receivable_instrument_settlements (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL,
    instrument_id uuid NOT NULL,
    allocation_id uuid NOT NULL,
    receivable_id uuid NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT receivable_instrument_settlements_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT receivable_instrument_settlements_instrument_fk FOREIGN KEY (organization_id, instrument_id, receivable_id, currency)
        REFERENCES biznes.receivable_instruments (organization_id, id, receivable_id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT receivable_instrument_settlements_allocation_fk FOREIGN KEY (organization_id, allocation_id, receivable_id, currency)
        REFERENCES biznes.receivable_allocations (organization_id, id, receivable_id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT receivable_instrument_settlements_amount_valid CHECK (amount > 0),
    CONSTRAINT receivable_instrument_settlements_currency_valid CHECK (currency = 'IRR')
);

CREATE INDEX receivable_instrument_settlements_instrument_created_id_idx
    ON biznes.receivable_instrument_settlements (organization_id, instrument_id, created_at, id);
CREATE INDEX receivable_instrument_settlements_allocation_idx
    ON biznes.receivable_instrument_settlements (organization_id, allocation_id);
CREATE INDEX receivable_instrument_settlements_created_by_idx ON biznes.receivable_instrument_settlements (created_by);

-- +goose Down
LOCK TABLE biznes.receivable_instrument_settlements IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.receivable_instrument_settlements) THEN
        RAISE EXCEPTION 'receivable instrument settlements migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.receivable_instrument_settlements RESTRICT;
ALTER TABLE biznes.receivable_allocations DROP CONSTRAINT receivable_allocations_settlement_reference_unique RESTRICT;
ALTER TABLE biznes.receivable_instruments DROP CONSTRAINT receivable_instruments_settlement_reference_unique RESTRICT;
