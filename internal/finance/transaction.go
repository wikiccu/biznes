package finance

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type Transaction struct {
	ID, OrganizationID, IdempotencyKey, AccountID, CategoryID, Kind, Currency, Description, CreatedBy string
	Amount                                                                                            int64
	OccurredAt, CreatedAt                                                                             time.Time
}

type TransactionInput struct {
	IdempotencyKey, AccountID, CategoryID, Amount, Currency, OccurredAt, Description string
}

var (
	ErrTransactionConflict = errors.New("transaction idempotency key already used with different input")
	transactionUUID        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	transactionAmount      = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	transactionTime        = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,6})?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)
)

func validateTransaction(input TransactionInput) (int64, time.Time, error) {
	var fields []FieldError
	for _, field := range []struct{ name, value string }{
		{"idempotency_key", input.IdempotencyKey}, {"account_id", input.AccountID}, {"category_id", input.CategoryID},
	} {
		if field.value == "" {
			fields = append(fields, FieldError{field.name, "required"})
		} else if !transactionUUID.MatchString(field.value) {
			fields = append(fields, FieldError{field.name, "invalid_format"})
		}
	}
	amount, err := strconv.ParseInt(input.Amount, 10, 64)
	switch {
	case input.Amount == "":
		fields = append(fields, FieldError{"amount", "required"})
	case !transactionAmount.MatchString(input.Amount):
		fields = append(fields, FieldError{"amount", "invalid_format"})
	case err != nil || amount <= 0:
		fields = append(fields, FieldError{"amount", "out_of_range"})
	}
	if input.Currency == "" {
		fields = append(fields, FieldError{"currency", "required"})
	} else if input.Currency != "IRR" {
		fields = append(fields, FieldError{"currency", "invalid_format"})
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, input.OccurredAt)
	occurredAt = occurredAt.UTC()
	switch {
	case input.OccurredAt == "":
		fields = append(fields, FieldError{"occurred_at", "required"})
	case err != nil || !transactionTime.MatchString(input.OccurredAt) || occurredAt.Year() < 1 || occurredAt.Year() > 9999:
		fields = append(fields, FieldError{"occurred_at", "invalid_format"})
	}
	if !utf8.ValidString(input.Description) || strings.ContainsFunc(input.Description, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
	}) {
		fields = append(fields, FieldError{"description", "invalid_format"})
	} else if utf8.RuneCountInString(input.Description) > 2000 {
		fields = append(fields, FieldError{"description", "out_of_range"})
	}
	if len(fields) != 0 {
		return 0, time.Time{}, &ValidationError{fields}
	}
	return amount, occurredAt, nil
}

func scanTransaction(row pgx.Row) (Transaction, error) {
	var item Transaction
	err := row.Scan(&item.ID, &item.OrganizationID, &item.IdempotencyKey, &item.AccountID, &item.CategoryID,
		&item.Kind, &item.Amount, &item.Currency, &item.OccurredAt, &item.Description, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) RecordTransaction(ctx context.Context, userID, organizationID string, input TransactionInput) (Transaction, bool, error) {
	amount, occurredAt, err := validateTransaction(input)
	if err != nil {
		return Transaction{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Transaction{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Transaction{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Transaction{}, false, organization.ErrForbidden
	}
	item, err := scanTransaction(tx.QueryRow(ctx, `INSERT INTO biznes.transactions
		(organization_id, idempotency_key, account_id, category_id, kind, amount, currency, occurred_at, description, created_by)
		SELECT a.organization_id, $2, a.id, c.id, c.kind, $5, a.currency, $7, $8, $9
		FROM biznes.financial_accounts a JOIN biznes.transaction_categories c ON c.organization_id = a.organization_id
		WHERE a.organization_id = $1 AND a.id = $3 AND c.id = $4 AND a.currency = $6
		ON CONFLICT (organization_id, idempotency_key) DO NOTHING
		RETURNING id, organization_id, idempotency_key, account_id, category_id, kind, amount, currency,
		occurred_at, description, created_by, created_at`,
		organizationID, input.IdempotencyKey, input.AccountID, input.CategoryID, amount, input.Currency, occurredAt, input.Description, userID))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// A fresh statement sees the winner after a concurrent ON CONFLICT wait.
		item, err = scanTransaction(tx.QueryRow(ctx, `SELECT id, organization_id, idempotency_key, account_id, category_id,
			kind, amount, currency, occurred_at, description, created_by, created_at
			FROM biznes.transactions WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, input.IdempotencyKey))
		if errors.Is(err, pgx.ErrNoRows) {
			return Transaction{}, false, organization.ErrNotFound
		}
		if err == nil && (item.AccountID != input.AccountID || item.CategoryID != input.CategoryID || item.Amount != amount ||
			item.Currency != input.Currency || !item.OccurredAt.Equal(occurredAt) || item.Description != input.Description) {
			return Transaction{}, false, ErrTransactionConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Transaction{}, false, organization.ErrNotFound
		}
		return Transaction{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Transaction{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetTransaction(ctx context.Context, userID, organizationID, transactionID string) (Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanTransaction(s.pool.QueryRow(ctx, `SELECT t.id, t.organization_id, t.idempotency_key, t.account_id, t.category_id,
		t.kind, t.amount, t.currency, t.occurred_at, t.description, t.created_by, t.created_at
		FROM biznes.transactions t JOIN biznes.memberships m ON m.organization_id = t.organization_id
		WHERE t.organization_id = $1 AND t.id = $2 AND m.user_id = $3`, organizationID, transactionID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, organization.ErrNotFound
	}
	if err != nil {
		return Transaction{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListTransactions(ctx context.Context, userID, organizationID string, page, limit int) ([]Transaction, error) {
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
	rows, err := tx.Query(ctx, `SELECT id, organization_id, idempotency_key, account_id, category_id,
		kind, amount, currency, occurred_at, description, created_by, created_at
		FROM biznes.transactions WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`,
		organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Transaction, 0)
	for rows.Next() {
		item, err := scanTransaction(rows)
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
