package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

var ErrPayableAllocationConflict = errors.New("payable allocation conflicts with payment, outstanding amount, or retry input")

func (s *Service) AllocatePayable(ctx context.Context, userID, organizationID, payableID string, input AllocationInput) (Allocation, bool, error) {
	amount, err := validateAllocation(input)
	if err != nil {
		return Allocation{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Allocation{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Allocation{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Allocation{}, false, organization.ErrForbidden
	}
	// Always lock the payment before the debt; reversals share the payment lock.
	if lockTransactionRecognition(ctx, tx, organizationID, input.TransactionID) != nil {
		return Allocation{}, false, ErrUnavailable
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(
		'biznes:payable:' || $1::uuid::text || ':' || $2::uuid::text, 0))`, organizationID, payableID); err != nil {
		return Allocation{}, false, ErrUnavailable
	}
	// A fresh statement after waiting sees committed allocations and reversals.
	item, err := scanAllocation(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, payable_id,
		transaction_id, amount, currency, created_by, created_at FROM biznes.payable_allocations
		WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
	created := false
	if errors.Is(err, pgx.ErrNoRows) {
		var currency string
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT r.currency,
			t.kind = 'expense' AND t.currency = r.currency
			AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
				WHERE v.organization_id = t.organization_id AND v.transaction_id = t.id)
			AND $4::numeric <= r.amount - (
				SELECT COALESCE(SUM(a.amount), 0) FROM biznes.payable_allocations a
				WHERE a.organization_id = r.organization_id AND a.payable_id = r.id
					AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
						WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id))
			AND $4::numeric <= t.amount - (
				SELECT COALESCE(SUM(a.amount), 0) FROM biznes.payable_allocations a
				WHERE a.organization_id = t.organization_id AND a.transaction_id = t.id)
			FROM biznes.payables r JOIN biznes.transactions t ON t.organization_id = r.organization_id
			WHERE r.organization_id = $1 AND r.id = $2 AND t.id = $3`,
			organizationID, payableID, input.TransactionID, input.Amount).Scan(&currency, &eligible)
		if errors.Is(err, pgx.ErrNoRows) {
			return Allocation{}, false, organization.ErrNotFound
		}
		if err != nil {
			return Allocation{}, false, ErrUnavailable
		}
		if !eligible {
			return Allocation{}, false, ErrPayableAllocationConflict
		}
		item, err = scanAllocation(tx.QueryRow(ctx, `INSERT INTO biznes.payable_allocations
			(organization_id, idempotency_key, payable_id, transaction_id, amount, currency, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (organization_id, idempotency_key) DO NOTHING
			RETURNING id, organization_id, idempotency_key, payable_id, transaction_id, amount, currency, created_by, created_at`,
			organizationID, input.IdempotencyKey, payableID, input.TransactionID, amount, currency, userID))
		created = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			// Different payment/debt pairs can still compete for the same organization/key.
			item, err = scanAllocation(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, payable_id,
				transaction_id, amount, currency, created_by, created_at FROM biznes.payable_allocations
				WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Allocation{}, false, organization.ErrNotFound
		}
		return Allocation{}, false, ErrUnavailable
	}
	if item.DebtID != payableID || item.TransactionID != input.TransactionID || item.Amount != amount {
		return Allocation{}, false, ErrPayableAllocationConflict
	}
	if tx.Commit(ctx) != nil {
		return Allocation{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetPayableAllocation(ctx context.Context, userID, organizationID, payableID, allocationID string) (Allocation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanAllocation(s.pool.QueryRow(ctx, `SELECT a.id, a.organization_id, a.idempotency_key, a.payable_id,
		a.transaction_id, a.amount, a.currency, a.created_by, a.created_at
		FROM biznes.payable_allocations a JOIN biznes.memberships m ON m.organization_id = a.organization_id
		WHERE a.organization_id = $1 AND a.payable_id = $2 AND a.id = $3 AND m.user_id = $4`,
		organizationID, payableID, allocationID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Allocation{}, organization.ErrNotFound
	}
	if err != nil {
		return Allocation{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListPayableAllocations(ctx context.Context, userID, organizationID, payableID string, page, limit int) ([]Allocation, error) {
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
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM biznes.payables WHERE organization_id = $1 AND id = $2)`,
		organizationID, payableID).Scan(&exists); err != nil {
		return nil, ErrUnavailable
	}
	if !exists {
		return nil, organization.ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, payable_id,
		transaction_id, amount, currency, created_by, created_at FROM biznes.payable_allocations
		WHERE organization_id = $1 AND payable_id = $2 ORDER BY created_at, id LIMIT $3 OFFSET $4`,
		organizationID, payableID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Allocation, 0)
	for rows.Next() {
		item, err := scanAllocation(rows)
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

func (s *Service) GetPayablePaymentSummary(ctx context.Context, userID, organizationID, payableID string) (AllocationSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var item AllocationSummary
	var valid bool
	// ponytail: derive totals from allocation history; add rollups only if measured latency requires them.
	err := s.pool.QueryRow(ctx, `SELECT r.organization_id, r.id, r.currency, r.amount::text,
		totals.paid::text, (r.amount - totals.paid)::text, totals.paid <= r.amount
		FROM biznes.payables r JOIN biznes.memberships m ON m.organization_id = r.organization_id
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(a.amount), 0) AS paid FROM biznes.payable_allocations a
			WHERE a.organization_id = r.organization_id AND a.payable_id = r.id
				AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
					WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)
		) totals WHERE r.organization_id = $1 AND r.id = $2 AND m.user_id = $3`, organizationID, payableID, userID).
		Scan(&item.OrganizationID, &item.DebtID, &item.Currency, &item.Amount, &item.AllocatedAmount, &item.OutstandingAmount, &valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return AllocationSummary{}, organization.ErrNotFound
	}
	if err != nil || !valid {
		return AllocationSummary{}, ErrUnavailable
	}
	return item, nil
}
