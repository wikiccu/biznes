package finance

import (
	"context"
	"time"
)

func (s *Service) ListUpcomingReceivables(ctx context.Context, userID, organizationID string, page, limit int, input DueDateRange) ([]UpcomingDebt, error) {
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
		SELECT p.id, p.contact_id, p.amount, p.currency, p.due_date, collections.collected,
			p.amount - collections.collected AS outstanding
		FROM authorized m JOIN biznes.receivables p ON p.organization_id = m.organization_id
		CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(a.amount), 0) AS collected FROM biznes.receivable_allocations a
			WHERE a.organization_id = p.organization_id AND a.receivable_id = p.id
				AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
					WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)
		) collections WHERE p.due_date >= $3::date AND p.due_date <= $4::date
	), selected AS (
		SELECT * FROM balances WHERE outstanding > 0 ORDER BY due_date, id LIMIT $5 OFFSET $6
	)
	SELECT NOT EXISTS (SELECT 1 FROM balances WHERE outstanding < 0),
		COALESCE(p.id::text, ''), COALESCE(p.contact_id::text, ''), COALESCE(p.amount::text, '0'),
		COALESCE(p.currency, 'IRR'), COALESCE(p.due_date, DATE '0001-01-01'),
		COALESCE(p.collected::text, '0'), COALESCE(p.outstanding::text, '0')
	FROM authorized m LEFT JOIN selected p ON true ORDER BY p.due_date, p.id`,
		organizationID, userID, from, to, limit, (page-1)*limit))
}
