package finance

import (
	"context"
	"time"

	"github.com/wikiccu/biznes/internal/organization"
)

func (s *Service) ListUpcomingObligations(ctx context.Context, userID, organizationID string, page, limit int, input DueDateRange) ([]Obligation, error) {
	from, to, err := validateDueDateRange(input)
	if err != nil {
		return nil, err
	}
	if page < 1 || page > 10000 || limit < 1 || limit > 100 {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// shortcut: select recorded commitments on demand; add a due-date index if measured latency requires it.
	rows, err := s.pool.Query(ctx, `WITH authorized AS (
		SELECT organization_id FROM biznes.memberships WHERE organization_id = $1 AND user_id = $2
	), selected AS (
		SELECT o.* FROM biznes.financial_obligations o JOIN authorized m ON m.organization_id = o.organization_id
		WHERE o.due_date >= $3::date AND o.due_date <= $4::date
			AND NOT EXISTS (SELECT 1 FROM biznes.financial_obligation_cancellations c
				WHERE c.organization_id = o.organization_id AND c.obligation_id = o.id)
		ORDER BY o.due_date, o.id LIMIT $5 OFFSET $6
	)
	SELECT COALESCE(o.id::text, ''), m.organization_id, COALESCE(o.idempotency_key::text, ''),
		COALESCE(o.title, ''), COALESCE(o.amount, 0), COALESCE(o.currency, 'IRR'),
		COALESCE(o.due_date, DATE '0001-01-01'), COALESCE(o.description, ''),
		COALESCE(o.created_by::text, ''), COALESCE(o.created_at, TIMESTAMPTZ '0001-01-01 00:00:00+00'), 'recorded'::text
	FROM authorized m LEFT JOIN selected o ON true ORDER BY o.due_date, o.id`,
		organizationID, userID, from, to, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Obligation, 0)
	found := false
	for rows.Next() {
		item, err := scanObligation(rows)
		if err != nil {
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
