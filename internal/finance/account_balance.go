package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wikiccu/biznes/internal/organization"
)

type AccountBalance struct {
	OrganizationID, AccountID, Currency, OpeningBalanceID, OpeningAmount string
	IncomeAmount, ExpenseAmount, NetAmount, BalanceAmount                string
	EffectiveAt, AsOf                                                    time.Time
}

var ErrAccountOpeningRequired = errors.New("an account opening effective by the cutoff is required")

func (s *Service) GetAccountBalance(ctx context.Context, userID, organizationID, accountID, asOf string) (AccountBalance, error) {
	var cutoff *time.Time
	if asOf != "" {
		instant, valid := parseTransactionTime(asOf)
		if !valid {
			return AccountBalance{}, &ValidationError{[]FieldError{{"as_of", "invalid_format"}}}
		}
		cutoff = &instant
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var item AccountBalance
	var effectiveAt *time.Time
	var validCutoff bool
	// shortcut: aggregate recorded history on demand; add rollups only if measured latency requires them.
	err := s.pool.QueryRow(ctx, `SELECT a.organization_id, a.id, a.currency,
		COALESCE(b.id::text, ''), COALESCE(b.amount, 0)::text, b.effective_at, cutoff.at,
		totals.income::text, totals.expense::text, (totals.income - totals.expense)::text,
		(COALESCE(b.amount, 0)::numeric + totals.income - totals.expense)::text,
		cutoff.at <= statement_timestamp()
		FROM biznes.financial_accounts a JOIN biznes.memberships m ON m.organization_id = a.organization_id
		LEFT JOIN biznes.account_opening_balances b
			ON b.organization_id = a.organization_id AND b.account_id = a.id AND b.currency = a.currency
		CROSS JOIN (SELECT COALESCE($4::timestamptz, statement_timestamp()) AS at) cutoff
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(t.amount) FILTER (WHERE t.kind = 'income'), 0) AS income,
				COALESCE(SUM(t.amount) FILTER (WHERE t.kind = 'expense'), 0) AS expense
			FROM biznes.transactions t
			WHERE t.organization_id = a.organization_id AND t.account_id = a.id AND t.currency = a.currency
				AND t.occurred_at >= b.effective_at AND t.occurred_at <= cutoff.at
				AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals r
					WHERE r.organization_id = t.organization_id AND r.transaction_id = t.id)
		) totals
		WHERE a.organization_id = $1 AND a.id = $2 AND m.user_id = $3`, organizationID, accountID, userID, cutoff).
		Scan(&item.OrganizationID, &item.AccountID, &item.Currency, &item.OpeningBalanceID, &item.OpeningAmount,
			&effectiveAt, &item.AsOf, &item.IncomeAmount, &item.ExpenseAmount, &item.NetAmount, &item.BalanceAmount, &validCutoff)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountBalance{}, organization.ErrNotFound
	}
	if err != nil {
		return AccountBalance{}, ErrUnavailable
	}
	if !validCutoff {
		return AccountBalance{}, &ValidationError{[]FieldError{{"as_of", "out_of_range"}}}
	}
	if effectiveAt == nil || effectiveAt.After(item.AsOf) {
		return AccountBalance{}, ErrAccountOpeningRequired
	}
	item.EffectiveAt = *effectiveAt
	return item, nil
}
