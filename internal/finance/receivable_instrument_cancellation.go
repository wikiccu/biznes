package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

func (s *Service) CancelReceivableInstrument(ctx context.Context, userID, organizationID, instrumentID string, input InstrumentCancellationInput) (InstrumentCancellation, bool, error) {
	input, err := validateReversal(input)
	if err != nil {
		return InstrumentCancellation{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return InstrumentCancellation{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return InstrumentCancellation{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return InstrumentCancellation{}, false, organization.ErrForbidden
	}
	if lockReceivableInstrument(ctx, tx, organizationID, instrumentID) != nil {
		return InstrumentCancellation{}, false, ErrUnavailable
	}
	item, err := scanInstrumentCancellation(tx.QueryRow(ctx, `INSERT INTO biznes.receivable_instrument_cancellations
		(organization_id, instrument_id, idempotency_key, reason, created_by)
		SELECT i.organization_id, i.id, $3, $4, $5 FROM biznes.receivable_instruments i WHERE i.organization_id = $1 AND i.id = $2
		AND NOT EXISTS (SELECT 1 FROM biznes.receivable_instrument_settlements s
			WHERE s.organization_id = i.organization_id AND s.instrument_id = i.id)
		ON CONFLICT DO NOTHING
		RETURNING id, organization_id, instrument_id, idempotency_key, reason, created_by, created_at`,
		organizationID, instrumentID, input.IdempotencyKey, input.Reason, userID))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// Read the committed winner after either unique constraint serialized concurrent requests.
		item, err = scanInstrumentCancellation(tx.QueryRow(ctx, `SELECT id, organization_id, instrument_id, idempotency_key, reason, created_by, created_at
			FROM biznes.receivable_instrument_cancellations WHERE organization_id = $1 AND (idempotency_key = $3 OR instrument_id = $2)
			ORDER BY (idempotency_key = $3) DESC LIMIT 1`, organizationID, instrumentID, input.IdempotencyKey))
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM biznes.receivable_instruments WHERE organization_id = $1 AND id = $2)`,
				organizationID, instrumentID).Scan(&exists); err != nil {
				return InstrumentCancellation{}, false, ErrUnavailable
			}
			if exists {
				return InstrumentCancellation{}, false, ErrInstrumentHasSettlements
			}
			return InstrumentCancellation{}, false, organization.ErrNotFound
		}
		if err == nil && (item.InstrumentID != instrumentID || item.IdempotencyKey != input.IdempotencyKey || item.Reason != input.Reason) {
			return InstrumentCancellation{}, false, ErrInstrumentCancellationConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return InstrumentCancellation{}, false, organization.ErrNotFound
		}
		return InstrumentCancellation{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return InstrumentCancellation{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetReceivableInstrumentCancellation(ctx context.Context, userID, organizationID, instrumentID string) (InstrumentCancellation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanInstrumentCancellation(s.pool.QueryRow(ctx, `SELECT r.id, r.organization_id, r.instrument_id, r.idempotency_key, r.reason, r.created_by, r.created_at
		FROM biznes.receivable_instrument_cancellations r JOIN biznes.memberships m ON m.organization_id = r.organization_id
		WHERE r.organization_id = $1 AND r.instrument_id = $2 AND m.user_id = $3`, organizationID, instrumentID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return InstrumentCancellation{}, organization.ErrNotFound
	}
	if err != nil {
		return InstrumentCancellation{}, ErrUnavailable
	}
	return item, nil
}
