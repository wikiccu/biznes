-- +goose Up
ALTER TABLE biznes.payable_instruments ADD CONSTRAINT payable_instruments_settlement_reference_unique
    UNIQUE (organization_id, id, payable_id, currency);
ALTER TABLE biznes.payable_allocations ADD CONSTRAINT payable_allocations_settlement_reference_unique
    UNIQUE (organization_id, id, payable_id, currency);

CREATE TABLE biznes.payable_instrument_settlements (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL,
    instrument_id uuid NOT NULL,
    allocation_id uuid NOT NULL,
    payable_id uuid NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT payable_instrument_settlements_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT payable_instrument_settlements_instrument_fk FOREIGN KEY (organization_id, instrument_id, payable_id, currency)
        REFERENCES biznes.payable_instruments (organization_id, id, payable_id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT payable_instrument_settlements_allocation_fk FOREIGN KEY (organization_id, allocation_id, payable_id, currency)
        REFERENCES biznes.payable_allocations (organization_id, id, payable_id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT payable_instrument_settlements_amount_valid CHECK (amount > 0),
    CONSTRAINT payable_instrument_settlements_currency_valid CHECK (currency = 'IRR')
);

CREATE INDEX payable_instrument_settlements_instrument_created_id_idx
    ON biznes.payable_instrument_settlements (organization_id, instrument_id, created_at, id);
CREATE INDEX payable_instrument_settlements_allocation_idx
    ON biznes.payable_instrument_settlements (organization_id, allocation_id);
CREATE INDEX payable_instrument_settlements_created_by_idx ON biznes.payable_instrument_settlements (created_by);

-- +goose Down
LOCK TABLE biznes.payable_instrument_settlements IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.payable_instrument_settlements) THEN
        RAISE EXCEPTION 'payable instrument settlements migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.payable_instrument_settlements RESTRICT;
ALTER TABLE biznes.payable_allocations DROP CONSTRAINT payable_allocations_settlement_reference_unique RESTRICT;
ALTER TABLE biznes.payable_instruments DROP CONSTRAINT payable_instruments_settlement_reference_unique RESTRICT;
