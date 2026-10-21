package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type Obligation struct {
	ID, OrganizationID, IdempotencyKey, Title, Currency, Description, CreatedBy string
	Amount                                                                      int64
	DueDate, CreatedAt                                                          time.Time
}

type ObligationInput struct{ IdempotencyKey, Title, Amount, Currency, DueDate, Description string }

var ErrObligationConflict = errors.New("obligation idempotency key already used with different input")

func validateObligation(input ObligationInput) (ObligationInput, int64, time.Time, error) {
	var fields []FieldError
	if input.IdempotencyKey == "" {
		fields = append(fields, FieldError{"idempotency_key", "required"})
	} else if !transactionUUID.MatchString(input.IdempotencyKey) {
		fields = append(fields, FieldError{"idempotency_key", "invalid_format"})
	}
	var code string
	input.Title, code = validateName(input.Title)
	if code != "" {
		fields = append(fields, FieldError{"title", code})
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
	dueDate, code := validateDate(input.DueDate)
	if code != "" {
		fields = append(fields, FieldError{"due_date", code})
	}
	if code := validateDescription(input.Description); code != "" {
		fields = append(fields, FieldError{"description", code})
	}
	if len(fields) != 0 {
		return ObligationInput{}, 0, time.Time{}, &ValidationError{fields}
	}
	return input, amount, dueDate, nil
}

func scanObligation(row pgx.Row) (Obligation, error) {
	var item Obligation
	err := row.Scan(&item.ID, &item.OrganizationID, &item.IdempotencyKey, &item.Title,
		&item.Amount, &item.Currency, &item.DueDate, &item.Description, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) RecordObligation(ctx context.Context, userID, organizationID string, input ObligationInput) (Obligation, bool, error) {
	input, amount, dueDate, err := validateObligation(input)
	if err != nil {
		return Obligation{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Obligation{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Obligation{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Obligation{}, false, organization.ErrForbidden
	}
	item, err := scanObligation(tx.QueryRow(ctx, `INSERT INTO biznes.financial_obligations
		(organization_id, idempotency_key, title, amount, currency, due_date, description, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (organization_id, idempotency_key) DO NOTHING
		RETURNING id, organization_id, idempotency_key, title, amount, currency, due_date, description, created_by, created_at`,
		organizationID, input.IdempotencyKey, input.Title, amount, input.Currency, dueDate, input.Description, userID))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// A fresh statement sees the winner after a concurrent ON CONFLICT wait.
		item, err = scanObligation(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, title,
			amount, currency, due_date, description, created_by, created_at
			FROM biznes.financial_obligations WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
		if err == nil && (item.Title != input.Title || item.Amount != amount || item.Currency != input.Currency ||
			!item.DueDate.Equal(dueDate) || item.Description != input.Description) {
			return Obligation{}, false, ErrObligationConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Obligation{}, false, organization.ErrNotFound
		}
		return Obligation{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Obligation{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetObligation(ctx context.Context, userID, organizationID, obligationID string) (Obligation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanObligation(s.pool.QueryRow(ctx, `SELECT o.id, o.organization_id, o.idempotency_key, o.title,
		o.amount, o.currency, o.due_date, o.description, o.created_by, o.created_at
		FROM biznes.financial_obligations o JOIN biznes.memberships m ON m.organization_id = o.organization_id
		WHERE o.organization_id = $1 AND o.id = $2 AND m.user_id = $3`, organizationID, obligationID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Obligation{}, organization.ErrNotFound
	}
	if err != nil {
		return Obligation{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListObligations(ctx context.Context, userID, organizationID string, page, limit int) ([]Obligation, error) {
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
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, title,
		amount, currency, due_date, description, created_by, created_at
		FROM biznes.financial_obligations WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`,
		organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Obligation, 0)
	for rows.Next() {
		item, err := scanObligation(rows)
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
