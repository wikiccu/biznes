-- +goose Up
CREATE TABLE biznes.transaction_reversals (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    transaction_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    reason text NOT NULL,
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT transaction_reversals_transaction_unique UNIQUE (organization_id, transaction_id),
    CONSTRAINT transaction_reversals_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT transaction_reversals_transaction_fk FOREIGN KEY (organization_id, transaction_id)
        REFERENCES biznes.transactions (organization_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT transaction_reversals_reason_valid CHECK (char_length(reason) BETWEEN 1 AND 2000 AND reason !~ '[[:cntrl:]]')
);

CREATE INDEX transaction_reversals_created_by_idx ON biznes.transaction_reversals (created_by);

-- +goose Down
LOCK TABLE biznes.transaction_reversals IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.transaction_reversals) THEN
        RAISE EXCEPTION 'transaction reversals migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.transaction_reversals RESTRICT;
