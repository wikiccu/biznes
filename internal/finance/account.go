package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type Account struct {
	ID, OrganizationID, Name, Kind, Currency string
	CreatedAt, UpdatedAt                     time.Time
}

type AccountInput struct{ Name, Kind, Currency string }

type AccountActivity struct {
	OrganizationID, AccountID, Currency, IncomeAmount, ExpenseAmount, NetAmount string
	From, To                                                                    *time.Time
}

var ErrAccountConflict = errors.New("account name already exists")

func validateAccount(input AccountInput, create bool) (AccountInput, error) {
	var fields []FieldError
	var code string
	input.Name, code = validateName(input.Name)
	if code != "" {
		fields = append(fields, FieldError{"name", code})
	}
	if create {
		if input.Kind == "" {
			fields = append(fields, FieldError{"kind", "required"})
		} else if input.Kind != "cash" && input.Kind != "bank" {
			fields = append(fields, FieldError{"kind", "invalid_format"})
		}
		if input.Currency == "" {
			fields = append(fields, FieldError{"currency", "required"})
		} else if input.Currency != "IRR" {
			fields = append(fields, FieldError{"currency", "invalid_format"})
		}
	}
	if len(fields) != 0 {
		return AccountInput{}, &ValidationError{fields}
	}
	return input, nil
}

func scanAccount(row pgx.Row) (Account, error) {
	var item Account
	err := row.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Kind, &item.Currency, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Service) CreateAccount(ctx context.Context, userID, organizationID string, input AccountInput) (Account, error) {
	return s.writeAccount(ctx, userID, organizationID, "", input)
}

func (s *Service) RenameAccount(ctx context.Context, userID, organizationID, accountID, name string) (Account, error) {
	if accountID == "" {
		return Account{}, organization.ErrNotFound
	}
	return s.writeAccount(ctx, userID, organizationID, accountID, AccountInput{Name: name})
}

func (s *Service) writeAccount(ctx context.Context, userID, organizationID, accountID string, input AccountInput) (Account, error) {
	input, err := validateAccount(input, accountID == "")
	if err != nil {
		return Account{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Account{}, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Account{}, organization.ErrForbidden
	}
	var row pgx.Row
	if accountID == "" {
		row = tx.QueryRow(ctx, `INSERT INTO biznes.financial_accounts (organization_id, name, kind, currency)
			VALUES ($1, $2, $3, $4) RETURNING id, organization_id, name, kind, currency, created_at, updated_at`,
			organizationID, input.Name, input.Kind, input.Currency)
	} else {
		row = tx.QueryRow(ctx, `UPDATE biznes.financial_accounts SET name = $3, updated_at = clock_timestamp()
			WHERE organization_id = $1 AND id = $2 RETURNING id, organization_id, name, kind, currency, created_at, updated_at`,
			organizationID, accountID, input.Name)
	}
	item, err := scanAccount(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, organization.ErrNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "financial_accounts_name_unique" {
			return Account{}, ErrAccountConflict
		}
		return Account{}, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Account{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) GetAccount(ctx context.Context, userID, organizationID, accountID string) (Account, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanAccount(s.pool.QueryRow(ctx, `SELECT a.id, a.organization_id, a.name, a.kind, a.currency, a.created_at, a.updated_at
		FROM biznes.financial_accounts a JOIN biznes.memberships m ON m.organization_id = a.organization_id
		WHERE a.organization_id = $1 AND a.id = $2 AND m.user_id = $3`, organizationID, accountID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, organization.ErrNotFound
	}
	if err != nil {
		return Account{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) GetAccountActivity(ctx context.Context, userID, organizationID string, input TransactionFilter) (AccountActivity, error) {
	from, to, err := validateTransactionFilter(input)
	if err != nil {
		return AccountActivity{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var item AccountActivity
	// ponytail: aggregate the account's records on demand; add rollups only if measured latency requires them.
	err = s.pool.QueryRow(ctx, `SELECT a.organization_id, a.id, a.currency,
		totals.income::text, totals.expense::text, (totals.income - totals.expense)::text
		FROM biznes.financial_accounts a JOIN biznes.memberships m ON m.organization_id = a.organization_id
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(t.amount) FILTER (WHERE t.kind = 'income'), 0) AS income,
				COALESCE(SUM(t.amount) FILTER (WHERE t.kind = 'expense'), 0) AS expense
			FROM biznes.transactions t
			WHERE t.organization_id = a.organization_id AND t.account_id = a.id AND t.currency = a.currency
				AND ($4::timestamptz IS NULL OR t.occurred_at >= $4)
				AND ($5::timestamptz IS NULL OR t.occurred_at < $5)
				AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals r
					WHERE r.organization_id = t.organization_id AND r.transaction_id = t.id)
		) totals
		WHERE a.organization_id = $1 AND a.id = $2 AND m.user_id = $3`, organizationID, input.AccountID, userID, from, to).
		Scan(&item.OrganizationID, &item.AccountID, &item.Currency, &item.IncomeAmount, &item.ExpenseAmount, &item.NetAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountActivity{}, organization.ErrNotFound
	}
	if err != nil {
		return AccountActivity{}, ErrUnavailable
	}
	item.From, item.To = from, to
	return item, nil
}

func (s *Service) ListAccounts(ctx context.Context, userID, organizationID string, page, limit int) ([]Account, error) {
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
	rows, err := tx.Query(ctx, `SELECT id, organization_id, name, kind, currency, created_at, updated_at
		FROM biznes.financial_accounts WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`,
		organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Account, 0)
	for rows.Next() {
		item, err := scanAccount(rows)
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
