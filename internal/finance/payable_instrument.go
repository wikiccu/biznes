package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type PayableInstrument struct {
	ID, OrganizationID, IdempotencyKey, PayableID, Kind, Currency, BankName, Reference, Description, CreatedBy string
	Amount                                                                                                     int64
	DueDate, CreatedAt                                                                                         time.Time
}

type PayableInstrumentInput struct {
	IdempotencyKey, PayableID, Kind, Amount, Currency, DueDate, BankName, Reference, Description string
}

var ErrPayableInstrumentConflict = errors.New("payable instrument conflicts with payable or retry input")

func validatePayableInstrument(input PayableInstrumentInput) (PayableInstrumentInput, int64, time.Time, error) {
	var fields []FieldError
	for _, field := range []struct{ name, value string }{
		{"idempotency_key", input.IdempotencyKey}, {"payable_id", input.PayableID},
	} {
		if field.value == "" {
			fields = append(fields, FieldError{field.name, "required"})
		} else if !transactionUUID.MatchString(field.value) {
			fields = append(fields, FieldError{field.name, "invalid_format"})
		}
	}
	if input.Kind == "" {
		fields = append(fields, FieldError{"kind", "required"})
	} else if input.Kind != "check" {
		fields = append(fields, FieldError{"kind", "invalid_format"})
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
	input.BankName, code = validateName(input.BankName)
	if code != "" {
		fields = append(fields, FieldError{"bank_name", code})
	}
	input.Reference, code = validateName(input.Reference)
	if code != "" {
		fields = append(fields, FieldError{"reference", code})
	}
	if code := validateDescription(input.Description); code != "" {
		fields = append(fields, FieldError{"description", code})
	}
	if len(fields) != 0 {
		return PayableInstrumentInput{}, 0, time.Time{}, &ValidationError{fields}
	}
	return input, amount, dueDate, nil
}

func scanPayableInstrument(row pgx.Row) (PayableInstrument, error) {
	var item PayableInstrument
	err := row.Scan(&item.ID, &item.OrganizationID, &item.IdempotencyKey, &item.PayableID, &item.Kind,
		&item.Amount, &item.Currency, &item.DueDate, &item.BankName, &item.Reference,
		&item.Description, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) RecordPayableInstrument(ctx context.Context, userID, organizationID string, input PayableInstrumentInput) (PayableInstrument, bool, error) {
	input, amount, dueDate, err := validatePayableInstrument(input)
	if err != nil {
		return PayableInstrument{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PayableInstrument{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return PayableInstrument{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return PayableInstrument{}, false, organization.ErrForbidden
	}
	item, err := scanPayableInstrument(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, payable_id, kind,
		amount, currency, due_date, bank_name, reference, description, created_by, created_at
		FROM biznes.payable_instruments WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, input.IdempotencyKey))
	created := false
	if errors.Is(err, pgx.ErrNoRows) {
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT $3::bigint <= amount AND $4::text = currency
			FROM biznes.payables WHERE organization_id = $1 AND id = $2`,
			organizationID, input.PayableID, amount, input.Currency).Scan(&eligible)
		if errors.Is(err, pgx.ErrNoRows) {
			return PayableInstrument{}, false, organization.ErrNotFound
		}
		if err != nil {
			return PayableInstrument{}, false, ErrUnavailable
		}
		if !eligible {
			return PayableInstrument{}, false, ErrPayableInstrumentConflict
		}
		item, err = scanPayableInstrument(tx.QueryRow(ctx, `INSERT INTO biznes.payable_instruments
			(organization_id, idempotency_key, payable_id, kind, amount, currency, due_date, bank_name, reference, description, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (organization_id, idempotency_key) DO NOTHING
			RETURNING id, organization_id, idempotency_key, payable_id, kind, amount, currency, due_date,
				bank_name, reference, description, created_by, created_at`,
			organizationID, input.IdempotencyKey, input.PayableID, input.Kind, amount, input.Currency, dueDate,
			input.BankName, input.Reference, input.Description, userID))
		created = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			// A fresh statement observes the committed retry winner across replicas.
			item, err = scanPayableInstrument(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, payable_id, kind,
				amount, currency, due_date, bank_name, reference, description, created_by, created_at
				FROM biznes.payable_instruments WHERE organization_id = $1 AND idempotency_key = $2`,
				organizationID, input.IdempotencyKey))
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return PayableInstrument{}, false, organization.ErrNotFound
		}
		return PayableInstrument{}, false, ErrUnavailable
	}
	if item.PayableID != input.PayableID || item.Kind != input.Kind || item.Amount != amount ||
		item.Currency != input.Currency || !item.DueDate.Equal(dueDate) || item.BankName != input.BankName ||
		item.Reference != input.Reference || item.Description != input.Description {
		return PayableInstrument{}, false, ErrPayableInstrumentConflict
	}
	if tx.Commit(ctx) != nil {
		return PayableInstrument{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetPayableInstrument(ctx context.Context, userID, organizationID, instrumentID string) (PayableInstrument, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanPayableInstrument(s.pool.QueryRow(ctx, `SELECT i.id, i.organization_id, i.idempotency_key, i.payable_id, i.kind,
		i.amount, i.currency, i.due_date, i.bank_name, i.reference, i.description, i.created_by, i.created_at
		FROM biznes.payable_instruments i JOIN biznes.memberships m ON m.organization_id = i.organization_id
		WHERE i.organization_id = $1 AND i.id = $2 AND m.user_id = $3`, organizationID, instrumentID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PayableInstrument{}, organization.ErrNotFound
	}
	if err != nil {
		return PayableInstrument{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListPayableInstruments(ctx context.Context, userID, organizationID string, page, limit int) ([]PayableInstrument, error) {
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
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, payable_id, kind,
		amount, currency, due_date, bank_name, reference, description, created_by, created_at
		FROM biznes.payable_instruments WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`,
		organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]PayableInstrument, 0)
	for rows.Next() {
		item, err := scanPayableInstrument(rows)
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
