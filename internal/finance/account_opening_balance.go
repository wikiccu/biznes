package finance

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type AccountOpeningBalance struct {
	ID, OrganizationID, AccountID, IdempotencyKey, Currency, Reason, CreatedBy string
	Amount                                                                     int64
	EffectiveAt, CreatedAt                                                     time.Time
}

type AccountOpeningBalanceInput struct{ IdempotencyKey, Amount, Currency, EffectiveAt, Reason string }

var (
	ErrAccountOpeningBalanceConflict = errors.New("account opening balance conflicts with existing record")
	accountOpeningAmount             = regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`)
)

func validateAccountOpeningBalance(input AccountOpeningBalanceInput) (AccountOpeningBalanceInput, int64, time.Time, error) {
	reason, err := validateReversal(ReversalInput{IdempotencyKey: input.IdempotencyKey, Reason: input.Reason})
	if err != nil {
		return AccountOpeningBalanceInput{}, 0, time.Time{}, err
	}
	input.Reason = reason.Reason
	var fields []FieldError
	amount, err := strconv.ParseInt(input.Amount, 10, 64)
	switch {
	case input.Amount == "":
		fields = append(fields, FieldError{"amount", "required"})
	case !accountOpeningAmount.MatchString(input.Amount):
		fields = append(fields, FieldError{"amount", "invalid_format"})
	case err != nil:
		fields = append(fields, FieldError{"amount", "out_of_range"})
	}
	if input.Currency == "" {
		fields = append(fields, FieldError{"currency", "required"})
	} else if input.Currency != "IRR" {
		fields = append(fields, FieldError{"currency", "invalid_format"})
	}
	effectiveAt, valid := parseTransactionTime(input.EffectiveAt)
	if input.EffectiveAt == "" {
		fields = append(fields, FieldError{"effective_at", "required"})
	} else if !valid {
		fields = append(fields, FieldError{"effective_at", "invalid_format"})
	}
	if len(fields) != 0 {
		return AccountOpeningBalanceInput{}, 0, time.Time{}, &ValidationError{fields}
	}
	return input, amount, effectiveAt, nil
}

func scanAccountOpeningBalance(row pgx.Row) (AccountOpeningBalance, error) {
	var item AccountOpeningBalance
	err := row.Scan(&item.ID, &item.OrganizationID, &item.AccountID, &item.IdempotencyKey,
		&item.Amount, &item.Currency, &item.EffectiveAt, &item.Reason, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) RecordAccountOpeningBalance(ctx context.Context, userID, organizationID, accountID string, input AccountOpeningBalanceInput) (AccountOpeningBalance, bool, error) {
	input, amount, effectiveAt, err := validateAccountOpeningBalance(input)
	if err != nil {
		return AccountOpeningBalance{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AccountOpeningBalance{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return AccountOpeningBalance{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return AccountOpeningBalance{}, false, organization.ErrForbidden
	}
	item, err := scanAccountOpeningBalance(tx.QueryRow(ctx, `INSERT INTO biznes.account_opening_balances
		(organization_id, account_id, idempotency_key, amount, currency, effective_at, reason, created_by)
		SELECT organization_id, id, $3, $4, currency, $6, $7, $8 FROM biznes.financial_accounts
		WHERE organization_id = $1 AND id = $2 AND currency = $5
		ON CONFLICT DO NOTHING
		RETURNING id, organization_id, account_id, idempotency_key, amount, currency, effective_at, reason, created_by, created_at`,
		organizationID, accountID, input.IdempotencyKey, amount, input.Currency, effectiveAt, input.Reason, userID))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// Read the committed winner after either unique constraint serialized concurrent requests.
		item, err = scanAccountOpeningBalance(tx.QueryRow(ctx, `SELECT id, organization_id, account_id, idempotency_key,
			amount, currency, effective_at, reason, created_by, created_at
			FROM biznes.account_opening_balances WHERE organization_id = $1 AND (idempotency_key = $3 OR account_id = $2)
			ORDER BY (idempotency_key = $3) DESC LIMIT 1`, organizationID, accountID, input.IdempotencyKey))
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountOpeningBalance{}, false, organization.ErrNotFound
		}
		if err == nil && (item.AccountID != accountID || item.IdempotencyKey != input.IdempotencyKey || item.Amount != amount ||
			item.Currency != input.Currency || !item.EffectiveAt.Equal(effectiveAt) || item.Reason != input.Reason) {
			return AccountOpeningBalance{}, false, ErrAccountOpeningBalanceConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return AccountOpeningBalance{}, false, organization.ErrNotFound
		}
		return AccountOpeningBalance{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return AccountOpeningBalance{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetAccountOpeningBalance(ctx context.Context, userID, organizationID, accountID string) (AccountOpeningBalance, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanAccountOpeningBalance(s.pool.QueryRow(ctx, `SELECT b.id, b.organization_id, b.account_id, b.idempotency_key,
		b.amount, b.currency, b.effective_at, b.reason, b.created_by, b.created_at
		FROM biznes.account_opening_balances b JOIN biznes.memberships m ON m.organization_id = b.organization_id
		WHERE b.organization_id = $1 AND b.account_id = $2 AND m.user_id = $3`, organizationID, accountID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountOpeningBalance{}, organization.ErrNotFound
	}
	if err != nil {
		return AccountOpeningBalance{}, ErrUnavailable
	}
	return item, nil
}
