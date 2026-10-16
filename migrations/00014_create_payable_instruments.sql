-- +goose Up
CREATE TABLE biznes.payable_instruments (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL,
    payable_id uuid NOT NULL,
    kind text COLLATE "C" NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    due_date date NOT NULL,
    bank_name text NOT NULL,
    reference text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT payable_instruments_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT payable_instruments_payable_fk FOREIGN KEY (organization_id, payable_id, currency)
        REFERENCES biznes.payables (organization_id, id, currency) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT payable_instruments_kind_valid CHECK (kind = 'check'),
    CONSTRAINT payable_instruments_amount_valid CHECK (amount > 0),
    CONSTRAINT payable_instruments_currency_valid CHECK (currency = 'IRR'),
    CONSTRAINT payable_instruments_due_date_valid CHECK (due_date >= DATE '0001-01-01' AND due_date < DATE '10000-01-01'),
    CONSTRAINT payable_instruments_bank_name_valid CHECK (char_length(bank_name) BETWEEN 1 AND 120
        AND bank_name !~ '[[:cntrl:]]' AND bank_name ~ '[^[:space:]]'),
    CONSTRAINT payable_instruments_reference_valid CHECK (char_length(reference) BETWEEN 1 AND 120
        AND reference !~ '[[:cntrl:]]' AND reference ~ '[^[:space:]]'),
    CONSTRAINT payable_instruments_description_valid CHECK (char_length(description) <= 2000
        AND translate(description, E'\n\r\t', '') !~ '[[:cntrl:]]')
);

CREATE INDEX payable_instruments_organization_created_id_idx ON biznes.payable_instruments (organization_id, created_at, id);
CREATE INDEX payable_instruments_payable_idx ON biznes.payable_instruments (organization_id, payable_id, currency);
CREATE INDEX payable_instruments_created_by_idx ON biznes.payable_instruments (created_by);

-- +goose Down
LOCK TABLE biznes.payable_instruments IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.payable_instruments) THEN
        RAISE EXCEPTION 'payable instruments migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.payable_instruments RESTRICT;
