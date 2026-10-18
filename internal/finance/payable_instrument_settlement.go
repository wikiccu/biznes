package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type InstrumentSettlement struct {
	ID, OrganizationID, IdempotencyKey, InstrumentID, AllocationID, DebtID, Currency, CreatedBy string
	Amount                                                                                      int64
	CreatedAt                                                                                   time.Time
}

type InstrumentSettlementInput struct{ IdempotencyKey, AllocationID, Amount string }

var (
	ErrInstrumentSettlementConflict = errors.New("instrument settlement conflicts with allocation, capacity, cancellation, or retry input")
	ErrInstrumentHasSettlements     = errors.New("instrument has settlement history")
)

func validateInstrumentSettlement(input InstrumentSettlementInput) (int64, error) {
	var fields []FieldError
	for _, field := range []struct{ name, value string }{
		{"idempotency_key", input.IdempotencyKey}, {"allocation_id", input.AllocationID},
	} {
		if field.value == "" {
			fields = append(fields, FieldError{field.name, "required"})
		} else if !transactionUUID.MatchString(field.value) {
			fields = append(fields, FieldError{field.name, "invalid_format"})
		}
	}
	amount, code := validateAmount(input.Amount)
	if code != "" {
		fields = append(fields, FieldError{"amount", code})
	}
	if len(fields) != 0 {
		return 0, &ValidationError{fields}
	}
	return amount, nil
}

func scanInstrumentSettlement(row pgx.Row) (InstrumentSettlement, error) {
	var item InstrumentSettlement
	err := row.Scan(&item.ID, &item.OrganizationID, &item.IdempotencyKey, &item.InstrumentID,
		&item.AllocationID, &item.DebtID, &item.Amount, &item.Currency, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func lockPayableInstrument(ctx context.Context, tx pgx.Tx, organizationID, instrumentID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(
		'biznes:payable-instrument:' || $1::uuid::text || ':' || $2::uuid::text, 0))`, organizationID, instrumentID)
	return err
}

func (s *Service) SettlePayableInstrument(ctx context.Context, userID, organizationID, instrumentID string, input InstrumentSettlementInput) (InstrumentSettlement, bool, error) {
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
	err = tx.QueryRow(ctx, `SELECT transaction_id FROM biznes.payable_allocations WHERE organization_id = $1 AND id = $2`,
		organizationID, input.AllocationID).Scan(&transactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return InstrumentSettlement{}, false, organization.ErrNotFound
	}
	if err != nil {
		return InstrumentSettlement{}, false, ErrUnavailable
	}
	// The payment lock serializes allocation attribution and full voids; cancellation shares the instrument lock.
	if lockTransactionRecognition(ctx, tx, organizationID, transactionID) != nil || lockPayableInstrument(ctx, tx, organizationID, instrumentID) != nil {
		return InstrumentSettlement{}, false, ErrUnavailable
	}
	item, err := scanInstrumentSettlement(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, instrument_id,
		allocation_id, payable_id, amount, currency, created_by, created_at FROM biznes.payable_instrument_settlements
		WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
	created := false
	if errors.Is(err, pgx.ErrNoRows) {
		var payableID, currency string
		var eligible bool
		// Fresh statements after lock waits see committed links, cancellations and full voids.
		err = tx.QueryRow(ctx, `SELECT i.payable_id, i.currency,
			a.payable_id = i.payable_id AND a.currency = i.currency
			AND NOT EXISTS (SELECT 1 FROM biznes.payable_instrument_cancellations c
				WHERE c.organization_id = i.organization_id AND c.instrument_id = i.id)
			AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
				WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)
			AND $4::numeric <= i.amount - (SELECT COALESCE(SUM(s.amount), 0) FROM biznes.payable_instrument_settlements s
				WHERE s.organization_id = i.organization_id AND s.instrument_id = i.id)
			AND $4::numeric <= a.amount - (SELECT COALESCE(SUM(s.amount), 0) FROM biznes.payable_instrument_settlements s
				WHERE s.organization_id = a.organization_id AND s.allocation_id = a.id)
			FROM biznes.payable_instruments i JOIN biznes.payable_allocations a ON a.organization_id = i.organization_id
			WHERE i.organization_id = $1 AND i.id = $2 AND a.id = $3`, organizationID, instrumentID, input.AllocationID, input.Amount).
			Scan(&payableID, &currency, &eligible)
		if errors.Is(err, pgx.ErrNoRows) {
			return InstrumentSettlement{}, false, organization.ErrNotFound
		}
		if err != nil {
			return InstrumentSettlement{}, false, ErrUnavailable
		}
		if !eligible {
			return InstrumentSettlement{}, false, ErrInstrumentSettlementConflict
		}
		item, err = scanInstrumentSettlement(tx.QueryRow(ctx, `INSERT INTO biznes.payable_instrument_settlements
			(organization_id, idempotency_key, instrument_id, allocation_id, payable_id, amount, currency, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (organization_id, idempotency_key) DO NOTHING
			RETURNING id, organization_id, idempotency_key, instrument_id, allocation_id, payable_id, amount, currency, created_by, created_at`,
			organizationID, input.IdempotencyKey, instrumentID, input.AllocationID, payableID, amount, currency, userID))
		created = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			item, err = scanInstrumentSettlement(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, instrument_id,
				allocation_id, payable_id, amount, currency, created_by, created_at FROM biznes.payable_instrument_settlements
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

func (s *Service) GetPayableInstrumentSettlement(ctx context.Context, userID, organizationID, instrumentID, settlementID string) (InstrumentSettlement, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanInstrumentSettlement(s.pool.QueryRow(ctx, `SELECT s.id, s.organization_id, s.idempotency_key, s.instrument_id,
		s.allocation_id, s.payable_id, s.amount, s.currency, s.created_by, s.created_at
		FROM biznes.payable_instrument_settlements s JOIN biznes.memberships m ON m.organization_id = s.organization_id
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

func (s *Service) ListPayableInstrumentSettlements(ctx context.Context, userID, organizationID, instrumentID string, page, limit int) ([]InstrumentSettlement, error) {
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
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM biznes.payable_instruments WHERE organization_id = $1 AND id = $2)`,
		organizationID, instrumentID).Scan(&exists); err != nil {
		return nil, ErrUnavailable
	}
	if !exists {
		return nil, organization.ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, instrument_id, allocation_id, payable_id,
		amount, currency, created_by, created_at FROM biznes.payable_instrument_settlements
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
