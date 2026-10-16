package finance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wikiccu/biznes/internal/organization"
)

type DueDateRange struct{ From, To string }

type UpcomingDebt struct {
	ID, ContactID, Amount, Currency, AllocatedAmount, OutstandingAmount string
	DueDate                                                             time.Time
}

func validateDueDateRange(input DueDateRange) (time.Time, time.Time, error) {
	from, fromCode := validateDate(input.From)
	to, toCode := validateDate(input.To)
	var fields []FieldError
	if fromCode != "" {
		fields = append(fields, FieldError{"due_from", fromCode})
	}
	if toCode != "" {
		fields = append(fields, FieldError{"due_to", toCode})
	}
	if len(fields) == 0 && from.After(to) {
		fields = append(fields, FieldError{"due_to", "invalid_range"})
	}
	if len(fields) != 0 {
		return time.Time{}, time.Time{}, &ValidationError{fields}
	}
	return from, to, nil
}

func (s *Service) ListUpcomingPayables(ctx context.Context, userID, organizationID string, page, limit int, input DueDateRange) ([]UpcomingDebt, error) {
	from, to, err := validateDueDateRange(input)
	if err != nil {
		return nil, err
	}
	if page < 1 || page > 10000 || limit < 1 || limit > 100 {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// shortcut: derive range balances on demand, add indexes or rollups only if measured latency requires them.
	return scanUpcomingDebts(s.pool.Query(ctx, `WITH authorized AS (
		SELECT organization_id FROM biznes.memberships WHERE organization_id = $1 AND user_id = $2
	), balances AS MATERIALIZED (
		SELECT p.id, p.contact_id, p.amount, p.currency, p.due_date, payments.paid,
			p.amount - payments.paid AS outstanding
		FROM authorized m JOIN biznes.payables p ON p.organization_id = m.organization_id
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(a.amount), 0) AS paid FROM biznes.payable_allocations a
			WHERE a.organization_id = p.organization_id AND a.payable_id = p.id
				AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
					WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)
		) payments WHERE p.due_date >= $3::date AND p.due_date <= $4::date
	), selected AS (
		SELECT * FROM balances WHERE outstanding > 0 ORDER BY due_date, id LIMIT $5 OFFSET $6
	)
	SELECT NOT EXISTS (SELECT 1 FROM balances WHERE outstanding < 0),
		COALESCE(p.id::text, ''), COALESCE(p.contact_id::text, ''), COALESCE(p.amount::text, '0'),
		COALESCE(p.currency, 'IRR'), COALESCE(p.due_date, DATE '0001-01-01'),
		COALESCE(p.paid::text, '0'), COALESCE(p.outstanding::text, '0')
	FROM authorized m LEFT JOIN selected p ON true ORDER BY p.due_date, p.id`,
		organizationID, userID, from, to, limit, (page-1)*limit))
}

func scanUpcomingDebts(rows pgx.Rows, err error) ([]UpcomingDebt, error) {
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]UpcomingDebt, 0)
	found := false
	for rows.Next() {
		var item UpcomingDebt
		var valid bool
		if err := rows.Scan(&valid, &item.ID, &item.ContactID, &item.Amount, &item.Currency,
			&item.DueDate, &item.AllocatedAmount, &item.OutstandingAmount); err != nil || !valid {
			return nil, ErrUnavailable
		}
		found = true
		if item.ID != "" {
			items = append(items, item)
		}
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	if !found {
		return nil, organization.ErrNotFound
	}
	return items, nil
}
