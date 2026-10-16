-- +goose Up
CREATE TABLE biznes.receivable_instrument_cancellations (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    instrument_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    reason text NOT NULL,
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT receivable_instrument_cancellations_instrument_unique UNIQUE (organization_id, instrument_id),
    CONSTRAINT receivable_instrument_cancellations_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT receivable_instrument_cancellations_instrument_fk FOREIGN KEY (organization_id, instrument_id)
        REFERENCES biznes.receivable_instruments (organization_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT receivable_instrument_cancellations_reason_valid CHECK (char_length(reason) BETWEEN 1 AND 2000 AND reason !~ '[[:cntrl:]]' AND reason ~ '[^[:space:]]')
);

CREATE INDEX receivable_instrument_cancellations_created_by_idx ON biznes.receivable_instrument_cancellations (created_by);

-- +goose Down
LOCK TABLE biznes.receivable_instrument_cancellations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.receivable_instrument_cancellations) THEN
        RAISE EXCEPTION 'receivable instrument cancellations migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.receivable_instrument_cancellations RESTRICT;
