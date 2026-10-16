-- +goose Up
CREATE TABLE biznes.payable_instrument_cancellations (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    instrument_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    reason text NOT NULL,
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT payable_instrument_cancellations_instrument_unique UNIQUE (organization_id, instrument_id),
    CONSTRAINT payable_instrument_cancellations_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT payable_instrument_cancellations_instrument_fk FOREIGN KEY (organization_id, instrument_id)
        REFERENCES biznes.payable_instruments (organization_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT payable_instrument_cancellations_reason_valid CHECK (char_length(reason) BETWEEN 1 AND 2000 AND reason !~ '[[:cntrl:]]' AND reason ~ '[^[:space:]]')
);

CREATE INDEX payable_instrument_cancellations_created_by_idx ON biznes.payable_instrument_cancellations (created_by);

-- +goose Down
LOCK TABLE biznes.payable_instrument_cancellations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.payable_instrument_cancellations) THEN
        RAISE EXCEPTION 'payable instrument cancellations migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.payable_instrument_cancellations RESTRICT;
