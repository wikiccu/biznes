package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type Payable = Debt
type PayableInput = DebtInput

var ErrPayableConflict = errors.New("payable idempotency key already used with different input")

func (s *Service) RecordPayable(ctx context.Context, userID, organizationID string, input PayableInput) (Payable, bool, error) {
	amount, dueDate, err := validateDebt(input)
	if err != nil {
		return Payable{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Payable{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Payable{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Payable{}, false, organization.ErrForbidden
	}
	var kind string
	err = tx.QueryRow(ctx, `SELECT kind FROM biznes.contacts WHERE organization_id = $1 AND id = $2 FOR SHARE`,
		organizationID, input.ContactID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payable{}, false, organization.ErrNotFound
	}
	if err != nil {
		return Payable{}, false, ErrUnavailable
	}
	item, err := scanDebt(tx.QueryRow(ctx, `INSERT INTO biznes.payables
		(organization_id, idempotency_key, contact_id, amount, currency, due_date, description, created_by)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8 WHERE $9::text IN ('supplier', 'both')
		ON CONFLICT (organization_id, idempotency_key) DO NOTHING
		RETURNING id, organization_id, idempotency_key, contact_id, amount, currency, due_date, description, created_by, created_at`,
		organizationID, input.IdempotencyKey, input.ContactID, amount, input.Currency, dueDate, input.Description, userID, kind))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// A fresh statement sees a concurrent winner, even after the contact's classification changes.
		item, err = scanDebt(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, contact_id,
			amount, currency, due_date, description, created_by, created_at
			FROM biznes.payables WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
		if errors.Is(err, pgx.ErrNoRows) {
			return Payable{}, false, &ValidationError{[]FieldError{{"contact_id", "invalid_kind"}}}
		}
		if err == nil && (item.ContactID != input.ContactID || item.Amount != amount || item.Currency != input.Currency ||
			!item.DueDate.Equal(dueDate) || item.Description != input.Description) {
			return Payable{}, false, ErrPayableConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Payable{}, false, organization.ErrNotFound
		}
		return Payable{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Payable{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetPayable(ctx context.Context, userID, organizationID, payableID string) (Payable, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanDebt(s.pool.QueryRow(ctx, `SELECT r.id, r.organization_id, r.idempotency_key, r.contact_id,
		r.amount, r.currency, r.due_date, r.description, r.created_by, r.created_at
		FROM biznes.payables r JOIN biznes.memberships m ON m.organization_id = r.organization_id
		WHERE r.organization_id = $1 AND r.id = $2 AND m.user_id = $3`, organizationID, payableID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Payable{}, organization.ErrNotFound
	}
	if err != nil {
		return Payable{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListPayables(ctx context.Context, userID, organizationID string, page, limit int) ([]Payable, error) {
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
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, contact_id,
		amount, currency, due_date, description, created_by, created_at
		FROM biznes.payables WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`,
		organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Payable, 0)
	for rows.Next() {
		item, err := scanDebt(rows)
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
