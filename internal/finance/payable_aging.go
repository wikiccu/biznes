package finance

import (
	"context"
	"time"
)

func (s *Service) GetPayableAging(ctx context.Context, userID, organizationID, date string) (DebtAging, error) {
	asOf, code := validateDate(date)
	if code != "" {
		return DebtAging{}, &ValidationError{[]FieldError{{"as_of", code}}}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// shortcut: aggregate recorded history on demand, add rollups only if measured latency requires them.
	return scanDebtAging(s.pool.QueryRow(ctx, `SELECT m.organization_id, totals.outstanding::text, totals.overdue::text,
		totals.not_due::text, totals.due_today::text, totals.days_1_30::text,
		totals.days_31_60::text, totals.days_61_90::text, totals.over_90::text, totals.valid
		FROM biznes.memberships m CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(owed.amount), 0) AS outstanding,
				COALESCE(SUM(owed.amount) FILTER (WHERE owed.due_date < $3::date), 0) AS overdue,
				COALESCE(SUM(owed.amount) FILTER (WHERE owed.due_date > $3::date), 0) AS not_due,
				COALESCE(SUM(owed.amount) FILTER (WHERE owed.due_date = $3::date), 0) AS due_today,
				COALESCE(SUM(owed.amount) FILTER (WHERE $3::date - owed.due_date BETWEEN 1 AND 30), 0) AS days_1_30,
				COALESCE(SUM(owed.amount) FILTER (WHERE $3::date - owed.due_date BETWEEN 31 AND 60), 0) AS days_31_60,
				COALESCE(SUM(owed.amount) FILTER (WHERE $3::date - owed.due_date BETWEEN 61 AND 90), 0) AS days_61_90,
				COALESCE(SUM(owed.amount) FILTER (WHERE $3::date - owed.due_date > 90), 0) AS over_90,
				COALESCE(BOOL_AND(owed.amount >= 0), true) AS valid
			FROM (
				SELECT r.due_date, r.amount - payments.paid AS amount
				FROM biznes.payables r CROSS JOIN LATERAL (
					SELECT COALESCE(SUM(a.amount), 0) AS paid FROM biznes.payable_allocations a
					WHERE a.organization_id = r.organization_id AND a.payable_id = r.id
						AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
							WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)
				) payments WHERE r.organization_id = m.organization_id
			) owed
		) totals WHERE m.organization_id = $1 AND m.user_id = $2`, organizationID, userID, asOf), asOf)
}
