-- +goose Up
CREATE TABLE biznes.financial_obligations (
    organization_id uuid NOT NULL REFERENCES biznes.organizations(id) ON DELETE RESTRICT,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    idempotency_key uuid NOT NULL,
    title text COLLATE "C" NOT NULL,
    amount bigint NOT NULL,
    currency text COLLATE "C" NOT NULL,
    due_date date NOT NULL,
    description text NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT financial_obligations_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT financial_obligations_title_valid CHECK (char_length(title) BETWEEN 1 AND 120 AND title !~ '[[:cntrl:]]'),
    CONSTRAINT financial_obligations_amount_valid CHECK (amount > 0),
    CONSTRAINT financial_obligations_currency_valid CHECK (currency = 'IRR'),
    CONSTRAINT financial_obligations_due_date_valid CHECK (due_date >= DATE '0001-01-01' AND due_date < DATE '10000-01-01'),
    CONSTRAINT financial_obligations_description_valid CHECK (char_length(description) <= 2000
        AND translate(description, E'\n\r\t', '') !~ '[[:cntrl:]]')
);

CREATE INDEX financial_obligations_organization_created_id_idx ON biznes.financial_obligations (organization_id, created_at, id);
CREATE INDEX financial_obligations_created_by_idx ON biznes.financial_obligations (created_by);

-- +goose Down
LOCK TABLE biznes.financial_obligations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.financial_obligations) THEN
        RAISE EXCEPTION 'financial obligations migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.financial_obligations RESTRICT;
