-- +goose Up
CREATE TABLE biznes.payables (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL,
    contact_id uuid NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    due_date date NOT NULL,
    description text NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT payables_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT payables_contact_fk FOREIGN KEY (organization_id, contact_id)
        REFERENCES biznes.contacts (organization_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT payables_amount_valid CHECK (amount > 0),
    CONSTRAINT payables_currency_valid CHECK (currency = 'IRR'),
    CONSTRAINT payables_due_date_valid CHECK (due_date >= DATE '0001-01-01' AND due_date < DATE '10000-01-01'),
    CONSTRAINT payables_description_valid CHECK (char_length(description) <= 2000
        AND translate(description, E'\n\r\t', '') !~ '[[:cntrl:]]')
);

CREATE INDEX payables_organization_created_id_idx ON biznes.payables (organization_id, created_at, id);
CREATE INDEX payables_contact_idx ON biznes.payables (organization_id, contact_id);
CREATE INDEX payables_created_by_idx ON biznes.payables (created_by);

-- +goose Down
LOCK TABLE biznes.payables IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.payables) THEN
        RAISE EXCEPTION 'payables migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.payables RESTRICT;
