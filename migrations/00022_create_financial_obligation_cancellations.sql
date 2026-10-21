-- +goose Up
CREATE TABLE biznes.financial_obligation_cancellations (
    organization_id uuid NOT NULL,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    obligation_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    reason text NOT NULL,
    created_by uuid NOT NULL REFERENCES biznes.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, id),
    CONSTRAINT financial_obligation_cancellations_obligation_unique UNIQUE (organization_id, obligation_id),
    CONSTRAINT financial_obligation_cancellations_idempotency_unique UNIQUE (organization_id, idempotency_key),
    CONSTRAINT financial_obligation_cancellations_obligation_fk FOREIGN KEY (organization_id, obligation_id)
        REFERENCES biznes.financial_obligations (organization_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT financial_obligation_cancellations_reason_valid CHECK (char_length(reason) BETWEEN 1 AND 2000
        AND reason !~ '[[:cntrl:]]' AND reason ~ '[^[:space:]]')
);

CREATE INDEX financial_obligation_cancellations_created_by_idx ON biznes.financial_obligation_cancellations (created_by);

-- +goose Down
LOCK TABLE biznes.financial_obligation_cancellations IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM biznes.financial_obligation_cancellations) THEN
        RAISE EXCEPTION 'financial obligation cancellations migration rollback requires an empty table';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE biznes.financial_obligation_cancellations RESTRICT;
