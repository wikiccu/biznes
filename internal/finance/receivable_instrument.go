package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

var ErrReceivableInstrumentConflict = errors.New("receivable instrument conflicts with receivable or retry input")

func (s *Service) RecordReceivableInstrument(ctx context.Context, userID, organizationID string, input InstrumentInput) (Instrument, bool, error) {
	input, amount, dueDate, err := validateInstrument(input, "receivable_id")
	if err != nil {
		return Instrument{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Instrument{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Instrument{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Instrument{}, false, organization.ErrForbidden
	}
	item, err := scanInstrument(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, receivable_id, kind,
		amount, currency, due_date, bank_name, reference, description, created_by, created_at
		FROM biznes.receivable_instruments WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, input.IdempotencyKey))
	created := false
	if errors.Is(err, pgx.ErrNoRows) {
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT $3::bigint <= amount AND $4::text = currency
			FROM biznes.receivables WHERE organization_id = $1 AND id = $2`,
			organizationID, input.DebtID, amount, input.Currency).Scan(&eligible)
		if errors.Is(err, pgx.ErrNoRows) {
			return Instrument{}, false, organization.ErrNotFound
		}
		if err != nil {
			return Instrument{}, false, ErrUnavailable
		}
		if !eligible {
			return Instrument{}, false, ErrReceivableInstrumentConflict
		}
		item, err = scanInstrument(tx.QueryRow(ctx, `INSERT INTO biznes.receivable_instruments
			(organization_id, idempotency_key, receivable_id, kind, amount, currency, due_date, bank_name, reference, description, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (organization_id, idempotency_key) DO NOTHING
			RETURNING id, organization_id, idempotency_key, receivable_id, kind, amount, currency, due_date,
				bank_name, reference, description, created_by, created_at`,
			organizationID, input.IdempotencyKey, input.DebtID, input.Kind, amount, input.Currency, dueDate,
			input.BankName, input.Reference, input.Description, userID))
		created = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			// A fresh statement observes the committed retry winner across replicas.
			item, err = scanInstrument(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, receivable_id, kind,
				amount, currency, due_date, bank_name, reference, description, created_by, created_at
				FROM biznes.receivable_instruments WHERE organization_id = $1 AND idempotency_key = $2`,
				organizationID, input.IdempotencyKey))
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Instrument{}, false, organization.ErrNotFound
		}
		return Instrument{}, false, ErrUnavailable
	}
	if item.DebtID != input.DebtID || item.Kind != input.Kind || item.Amount != amount ||
		item.Currency != input.Currency || !item.DueDate.Equal(dueDate) || item.BankName != input.BankName ||
		item.Reference != input.Reference || item.Description != input.Description {
		return Instrument{}, false, ErrReceivableInstrumentConflict
	}
	if tx.Commit(ctx) != nil {
		return Instrument{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetReceivableInstrument(ctx context.Context, userID, organizationID, instrumentID string) (Instrument, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanInstrument(s.pool.QueryRow(ctx, `SELECT i.id, i.organization_id, i.idempotency_key, i.receivable_id, i.kind,
		i.amount, i.currency, i.due_date, i.bank_name, i.reference, i.description, i.created_by, i.created_at
		FROM biznes.receivable_instruments i JOIN biznes.memberships m ON m.organization_id = i.organization_id
		WHERE i.organization_id = $1 AND i.id = $2 AND m.user_id = $3`, organizationID, instrumentID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Instrument{}, organization.ErrNotFound
	}
	if err != nil {
		return Instrument{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListReceivableInstruments(ctx context.Context, userID, organizationID string, page, limit int) ([]Instrument, error) {
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
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, receivable_id, kind,
		amount, currency, due_date, bank_name, reference, description, created_by, created_at
		FROM biznes.receivable_instruments WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`,
		organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Instrument, 0)
	for rows.Next() {
		item, err := scanInstrument(rows)
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
