package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type ReceivableAllocation struct {
	ID, OrganizationID, IdempotencyKey, ReceivableID, TransactionID, Currency, CreatedBy string
	Amount                                                                               int64
	CreatedAt                                                                            time.Time
}

type ReceivableAllocationInput struct{ IdempotencyKey, TransactionID, Amount string }

type ReceivableCollectionSummary struct {
	OrganizationID, ReceivableID, Currency, Amount, CollectedAmount, OutstandingAmount string
}

var ErrAllocationConflict = errors.New("receivable allocation conflicts with receipt, outstanding amount, or retry input")

func validateAllocation(input ReceivableAllocationInput) (int64, error) {
	var fields []FieldError
	for _, field := range []struct{ name, value string }{
		{"idempotency_key", input.IdempotencyKey}, {"transaction_id", input.TransactionID},
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

func scanReceivableAllocation(row pgx.Row) (ReceivableAllocation, error) {
	var item ReceivableAllocation
	err := row.Scan(&item.ID, &item.OrganizationID, &item.IdempotencyKey, &item.ReceivableID,
		&item.TransactionID, &item.Amount, &item.Currency, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) AllocateReceivable(ctx context.Context, userID, organizationID, receivableID string, input ReceivableAllocationInput) (ReceivableAllocation, bool, error) {
	amount, err := validateAllocation(input)
	if err != nil {
		return ReceivableAllocation{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReceivableAllocation{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return ReceivableAllocation{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return ReceivableAllocation{}, false, organization.ErrForbidden
	}
	// Always lock the receipt before the debt; reversals share the receipt lock.
	if lockTransactionRecognition(ctx, tx, organizationID, input.TransactionID) != nil {
		return ReceivableAllocation{}, false, ErrUnavailable
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(
		'biznes:receivable:' || $1::uuid::text || ':' || $2::uuid::text, 0))`, organizationID, receivableID); err != nil {
		return ReceivableAllocation{}, false, ErrUnavailable
	}
	// A fresh statement after waiting sees committed allocations and reversals.
	item, err := scanReceivableAllocation(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, receivable_id,
		transaction_id, amount, currency, created_by, created_at FROM biznes.receivable_allocations
		WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
	created := false
	if errors.Is(err, pgx.ErrNoRows) {
		var currency string
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT r.currency,
			t.kind = 'income' AND t.currency = r.currency
			AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
				WHERE v.organization_id = t.organization_id AND v.transaction_id = t.id)
			AND $4::numeric <= r.amount - (
				SELECT COALESCE(SUM(a.amount), 0) FROM biznes.receivable_allocations a
				WHERE a.organization_id = r.organization_id AND a.receivable_id = r.id
					AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
						WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id))
			AND $4::numeric <= t.amount - (
				SELECT COALESCE(SUM(a.amount), 0) FROM biznes.receivable_allocations a
				WHERE a.organization_id = t.organization_id AND a.transaction_id = t.id)
			FROM biznes.receivables r JOIN biznes.transactions t ON t.organization_id = r.organization_id
			WHERE r.organization_id = $1 AND r.id = $2 AND t.id = $3`,
			organizationID, receivableID, input.TransactionID, input.Amount).Scan(&currency, &eligible)
		if errors.Is(err, pgx.ErrNoRows) {
			return ReceivableAllocation{}, false, organization.ErrNotFound
		}
		if err != nil {
			return ReceivableAllocation{}, false, ErrUnavailable
		}
		if !eligible {
			return ReceivableAllocation{}, false, ErrAllocationConflict
		}
		item, err = scanReceivableAllocation(tx.QueryRow(ctx, `INSERT INTO biznes.receivable_allocations
			(organization_id, idempotency_key, receivable_id, transaction_id, amount, currency, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (organization_id, idempotency_key) DO NOTHING
			RETURNING id, organization_id, idempotency_key, receivable_id, transaction_id, amount, currency, created_by, created_at`,
			organizationID, input.IdempotencyKey, receivableID, input.TransactionID, amount, currency, userID))
		created = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			// Different receipt/debt pairs can still compete for the same organization/key.
			item, err = scanReceivableAllocation(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, receivable_id,
				transaction_id, amount, currency, created_by, created_at FROM biznes.receivable_allocations
				WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ReceivableAllocation{}, false, organization.ErrNotFound
		}
		return ReceivableAllocation{}, false, ErrUnavailable
	}
	if item.ReceivableID != receivableID || item.TransactionID != input.TransactionID || item.Amount != amount {
		return ReceivableAllocation{}, false, ErrAllocationConflict
	}
	if tx.Commit(ctx) != nil {
		return ReceivableAllocation{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetReceivableAllocation(ctx context.Context, userID, organizationID, receivableID, allocationID string) (ReceivableAllocation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanReceivableAllocation(s.pool.QueryRow(ctx, `SELECT a.id, a.organization_id, a.idempotency_key, a.receivable_id,
		a.transaction_id, a.amount, a.currency, a.created_by, a.created_at
		FROM biznes.receivable_allocations a JOIN biznes.memberships m ON m.organization_id = a.organization_id
		WHERE a.organization_id = $1 AND a.receivable_id = $2 AND a.id = $3 AND m.user_id = $4`,
		organizationID, receivableID, allocationID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ReceivableAllocation{}, organization.ErrNotFound
	}
	if err != nil {
		return ReceivableAllocation{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListReceivableAllocations(ctx context.Context, userID, organizationID, receivableID string, page, limit int) ([]ReceivableAllocation, error) {
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
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM biznes.receivables WHERE organization_id = $1 AND id = $2)`,
		organizationID, receivableID).Scan(&exists); err != nil {
		return nil, ErrUnavailable
	}
	if !exists {
		return nil, organization.ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, receivable_id,
		transaction_id, amount, currency, created_by, created_at FROM biznes.receivable_allocations
		WHERE organization_id = $1 AND receivable_id = $2 ORDER BY created_at, id LIMIT $3 OFFSET $4`,
		organizationID, receivableID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]ReceivableAllocation, 0)
	for rows.Next() {
		item, err := scanReceivableAllocation(rows)
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

func (s *Service) GetReceivableCollectionSummary(ctx context.Context, userID, organizationID, receivableID string) (ReceivableCollectionSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var item ReceivableCollectionSummary
	var valid bool
	// ponytail: derive totals from allocation history; add rollups only if measured latency requires them.
	err := s.pool.QueryRow(ctx, `SELECT r.organization_id, r.id, r.currency, r.amount::text,
		totals.collected::text, (r.amount - totals.collected)::text, totals.collected <= r.amount
		FROM biznes.receivables r JOIN biznes.memberships m ON m.organization_id = r.organization_id
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(a.amount), 0) AS collected FROM biznes.receivable_allocations a
			WHERE a.organization_id = r.organization_id AND a.receivable_id = r.id
				AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
					WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)
		) totals WHERE r.organization_id = $1 AND r.id = $2 AND m.user_id = $3`, organizationID, receivableID, userID).
		Scan(&item.OrganizationID, &item.ReceivableID, &item.Currency, &item.Amount, &item.CollectedAmount, &item.OutstandingAmount, &valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReceivableCollectionSummary{}, organization.ErrNotFound
	}
	if err != nil || !valid {
		return ReceivableCollectionSummary{}, ErrUnavailable
	}
	return item, nil
}
