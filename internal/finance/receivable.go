package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type Receivable struct {
	ID, OrganizationID, IdempotencyKey, ContactID, Currency, Description, CreatedBy string
	Amount                                                                          int64
	DueDate, CreatedAt                                                              time.Time
}

type ReceivableInput struct {
	IdempotencyKey, ContactID, Amount, Currency, DueDate, Description string
}

var ErrReceivableConflict = errors.New("receivable idempotency key already used with different input")

func validateReceivable(input ReceivableInput) (int64, time.Time, error) {
	var fields []FieldError
	for _, field := range []struct{ name, value string }{
		{"idempotency_key", input.IdempotencyKey}, {"contact_id", input.ContactID},
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
	if input.Currency == "" {
		fields = append(fields, FieldError{"currency", "required"})
	} else if input.Currency != "IRR" {
		fields = append(fields, FieldError{"currency", "invalid_format"})
	}
	dueDate, err := time.Parse(time.DateOnly, input.DueDate)
	if input.DueDate == "" {
		fields = append(fields, FieldError{"due_date", "required"})
	} else if err != nil || len(input.DueDate) != 10 || dueDate.Year() < 1 {
		fields = append(fields, FieldError{"due_date", "invalid_format"})
	}
	if code := validateDescription(input.Description); code != "" {
		fields = append(fields, FieldError{"description", code})
	}
	if len(fields) != 0 {
		return 0, time.Time{}, &ValidationError{fields}
	}
	return amount, dueDate, nil
}

func scanReceivable(row pgx.Row) (Receivable, error) {
	var item Receivable
	err := row.Scan(&item.ID, &item.OrganizationID, &item.IdempotencyKey, &item.ContactID,
		&item.Amount, &item.Currency, &item.DueDate, &item.Description, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) RecordReceivable(ctx context.Context, userID, organizationID string, input ReceivableInput) (Receivable, bool, error) {
	amount, dueDate, err := validateReceivable(input)
	if err != nil {
		return Receivable{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Receivable{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Receivable{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Receivable{}, false, organization.ErrForbidden
	}
	var kind string
	err = tx.QueryRow(ctx, `SELECT kind FROM biznes.contacts WHERE organization_id = $1 AND id = $2 FOR SHARE`,
		organizationID, input.ContactID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receivable{}, false, organization.ErrNotFound
	}
	if err != nil {
		return Receivable{}, false, ErrUnavailable
	}
	item, err := scanReceivable(tx.QueryRow(ctx, `INSERT INTO biznes.receivables
		(organization_id, idempotency_key, contact_id, amount, currency, due_date, description, created_by)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8 WHERE $9::text IN ('customer', 'both')
		ON CONFLICT (organization_id, idempotency_key) DO NOTHING
		RETURNING id, organization_id, idempotency_key, contact_id, amount, currency, due_date, description, created_by, created_at`,
		organizationID, input.IdempotencyKey, input.ContactID, amount, input.Currency, dueDate, input.Description, userID, kind))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// A fresh statement sees a concurrent winner, even after the contact's classification changes.
		item, err = scanReceivable(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, contact_id,
			amount, currency, due_date, description, created_by, created_at
			FROM biznes.receivables WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
		if errors.Is(err, pgx.ErrNoRows) {
			return Receivable{}, false, &ValidationError{[]FieldError{{"contact_id", "invalid_kind"}}}
		}
		if err == nil && (item.ContactID != input.ContactID || item.Amount != amount || item.Currency != input.Currency ||
			!item.DueDate.Equal(dueDate) || item.Description != input.Description) {
			return Receivable{}, false, ErrReceivableConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Receivable{}, false, organization.ErrNotFound
		}
		return Receivable{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Receivable{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetReceivable(ctx context.Context, userID, organizationID, receivableID string) (Receivable, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanReceivable(s.pool.QueryRow(ctx, `SELECT r.id, r.organization_id, r.idempotency_key, r.contact_id,
		r.amount, r.currency, r.due_date, r.description, r.created_by, r.created_at
		FROM biznes.receivables r JOIN biznes.memberships m ON m.organization_id = r.organization_id
		WHERE r.organization_id = $1 AND r.id = $2 AND m.user_id = $3`, organizationID, receivableID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Receivable{}, organization.ErrNotFound
	}
	if err != nil {
		return Receivable{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListReceivables(ctx context.Context, userID, organizationID string, page, limit int) ([]Receivable, error) {
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
		FROM biznes.receivables WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`,
		organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Receivable, 0)
	for rows.Next() {
		item, err := scanReceivable(rows)
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
