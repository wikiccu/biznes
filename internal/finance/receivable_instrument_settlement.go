package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

func lockReceivableInstrument(ctx context.Context, tx pgx.Tx, organizationID, instrumentID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(
		'biznes:receivable-instrument:' || $1::uuid::text || ':' || $2::uuid::text, 0))`, organizationID, instrumentID)
	return err
}

func (s *Service) SettleReceivableInstrument(ctx context.Context, userID, organizationID, instrumentID string, input InstrumentSettlementInput) (InstrumentSettlement, bool, error) {
	amount, err := validateInstrumentSettlement(input)
	if err != nil {
		return InstrumentSettlement{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return InstrumentSettlement{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return InstrumentSettlement{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return InstrumentSettlement{}, false, organization.ErrForbidden
	}
	var transactionID string
	err = tx.QueryRow(ctx, `SELECT transaction_id FROM biznes.receivable_allocations WHERE organization_id = $1 AND id = $2`,
		organizationID, input.AllocationID).Scan(&transactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return InstrumentSettlement{}, false, organization.ErrNotFound
	}
	if err != nil {
		return InstrumentSettlement{}, false, ErrUnavailable
	}
	// The receipt lock serializes allocation attribution and full voids; cancellation shares the instrument lock.
	if lockTransactionRecognition(ctx, tx, organizationID, transactionID) != nil || lockReceivableInstrument(ctx, tx, organizationID, instrumentID) != nil {
		return InstrumentSettlement{}, false, ErrUnavailable
	}
	item, err := scanInstrumentSettlement(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, instrument_id,
		allocation_id, receivable_id, amount, currency, created_by, created_at FROM biznes.receivable_instrument_settlements
		WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
	created := false
	if errors.Is(err, pgx.ErrNoRows) {
		var receivableID, currency string
		var eligible bool
		// Fresh statements after lock waits see committed links, cancellations and full voids.
		err = tx.QueryRow(ctx, `SELECT i.receivable_id, i.currency,
			a.receivable_id = i.receivable_id AND a.currency = i.currency
			AND NOT EXISTS (SELECT 1 FROM biznes.receivable_instrument_cancellations c
				WHERE c.organization_id = i.organization_id AND c.instrument_id = i.id)
			AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
				WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)
			AND $4::numeric <= i.amount - (SELECT COALESCE(SUM(s.amount), 0) FROM biznes.receivable_instrument_settlements s
				WHERE s.organization_id = i.organization_id AND s.instrument_id = i.id)
			AND $4::numeric <= a.amount - (SELECT COALESCE(SUM(s.amount), 0) FROM biznes.receivable_instrument_settlements s
				WHERE s.organization_id = a.organization_id AND s.allocation_id = a.id)
			FROM biznes.receivable_instruments i JOIN biznes.receivable_allocations a ON a.organization_id = i.organization_id
			WHERE i.organization_id = $1 AND i.id = $2 AND a.id = $3`, organizationID, instrumentID, input.AllocationID, input.Amount).
			Scan(&receivableID, &currency, &eligible)
		if errors.Is(err, pgx.ErrNoRows) {
			return InstrumentSettlement{}, false, organization.ErrNotFound
		}
		if err != nil {
			return InstrumentSettlement{}, false, ErrUnavailable
		}
		if !eligible {
			return InstrumentSettlement{}, false, ErrInstrumentSettlementConflict
		}
		item, err = scanInstrumentSettlement(tx.QueryRow(ctx, `INSERT INTO biznes.receivable_instrument_settlements
			(organization_id, idempotency_key, instrument_id, allocation_id, receivable_id, amount, currency, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (organization_id, idempotency_key) DO NOTHING
			RETURNING id, organization_id, idempotency_key, instrument_id, allocation_id, receivable_id, amount, currency, created_by, created_at`,
			organizationID, input.IdempotencyKey, instrumentID, input.AllocationID, receivableID, amount, currency, userID))
		created = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			item, err = scanInstrumentSettlement(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, instrument_id,
				allocation_id, receivable_id, amount, currency, created_by, created_at FROM biznes.receivable_instrument_settlements
				WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return InstrumentSettlement{}, false, organization.ErrNotFound
		}
		return InstrumentSettlement{}, false, ErrUnavailable
	}
	if item.InstrumentID != instrumentID || item.AllocationID != input.AllocationID || item.Amount != amount {
		return InstrumentSettlement{}, false, ErrInstrumentSettlementConflict
	}
	if tx.Commit(ctx) != nil {
		return InstrumentSettlement{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetReceivableInstrumentSettlement(ctx context.Context, userID, organizationID, instrumentID, settlementID string) (InstrumentSettlement, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanInstrumentSettlement(s.pool.QueryRow(ctx, `SELECT s.id, s.organization_id, s.idempotency_key, s.instrument_id,
		s.allocation_id, s.receivable_id, s.amount, s.currency, s.created_by, s.created_at
		FROM biznes.receivable_instrument_settlements s JOIN biznes.memberships m ON m.organization_id = s.organization_id
		WHERE s.organization_id = $1 AND s.instrument_id = $2 AND s.id = $3 AND m.user_id = $4`,
		organizationID, instrumentID, settlementID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return InstrumentSettlement{}, organization.ErrNotFound
	}
	if err != nil {
		return InstrumentSettlement{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListReceivableInstrumentSettlements(ctx context.Context, userID, organizationID, instrumentID string, page, limit int) ([]InstrumentSettlement, error) {
	if page < 1 || page > 10000 || limit < 1 || limit > 100 {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if _, err := organization.LockMembership(ctx, tx, userID, organizationID); err != nil {
		return nil, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM biznes.receivable_instruments WHERE organization_id = $1 AND id = $2)`,
		organizationID, instrumentID).Scan(&exists); err != nil {
		return nil, ErrUnavailable
	}
	if !exists {
		return nil, organization.ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, instrument_id, allocation_id, receivable_id,
		amount, currency, created_by, created_at FROM biznes.receivable_instrument_settlements
		WHERE organization_id = $1 AND instrument_id = $2 ORDER BY created_at, id LIMIT $3 OFFSET $4`,
		organizationID, instrumentID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]InstrumentSettlement, 0)
	for rows.Next() {
		item, err := scanInstrumentSettlement(rows)
		if err != nil {
			return nil, ErrUnavailable
		}
		items = append(items, item)
	}
	if rows.Err() != nil || tx.Commit(ctx) != nil {
		return nil, ErrUnavailable
	}
	return items, nil
}

func (s *Service) GetReceivableInstrumentSettlementSummary(ctx context.Context, userID, organizationID, instrumentID string) (InstrumentSettlementSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return scanInstrumentSettlementSummary(s.pool.QueryRow(ctx, `SELECT i.organization_id, i.id, i.currency, i.amount::text,
		totals.linked::text, totals.recognized::text, (totals.linked - totals.recognized)::text,
		(i.amount - totals.linked)::text, totals.linked <= i.amount AND totals.allocations_valid
		AND (totals.linked = 0 OR NOT EXISTS (SELECT 1 FROM biznes.receivable_instrument_cancellations c
			WHERE c.organization_id = i.organization_id AND c.instrument_id = i.id))
		FROM biznes.receivable_instruments i JOIN biznes.memberships m ON m.organization_id = i.organization_id
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(s.amount), 0) AS linked,
				COALESCE(SUM(s.amount) FILTER (WHERE NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
					WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)), 0) AS recognized,
				COALESCE(BOOL_AND(a.amount >= (SELECT COALESCE(SUM(other.amount), 0)
					FROM biznes.receivable_instrument_settlements other
					WHERE other.organization_id = a.organization_id AND other.allocation_id = a.id)), true) AS allocations_valid
			FROM biznes.receivable_instrument_settlements s JOIN biznes.receivable_allocations a
				ON a.organization_id = s.organization_id AND a.id = s.allocation_id
			WHERE s.organization_id = i.organization_id AND s.instrument_id = i.id
		) totals WHERE i.organization_id = $1 AND i.id = $2 AND m.user_id = $3`, organizationID, instrumentID, userID))
}
