package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wikiccu/biznes/internal/organization"
)

type AgingBucket struct{ Bucket, OutstandingAmount string }

type DebtAging struct {
	OrganizationID, OutstandingAmount, OverdueAmount string
	AsOf                                             time.Time
	Buckets                                          []AgingBucket
}

func scanDebtAging(row pgx.Row, asOf time.Time) (DebtAging, error) {
	item := DebtAging{AsOf: asOf, Buckets: []AgingBucket{
		{Bucket: "not_due"}, {Bucket: "due_today"}, {Bucket: "overdue_1_30"},
		{Bucket: "overdue_31_60"}, {Bucket: "overdue_61_90"}, {Bucket: "overdue_over_90"},
	}}
	var valid bool
	err := row.Scan(&item.OrganizationID, &item.OutstandingAmount, &item.OverdueAmount,
		&item.Buckets[0].OutstandingAmount, &item.Buckets[1].OutstandingAmount, &item.Buckets[2].OutstandingAmount,
		&item.Buckets[3].OutstandingAmount, &item.Buckets[4].OutstandingAmount, &item.Buckets[5].OutstandingAmount, &valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return DebtAging{}, organization.ErrNotFound
	}
	if err != nil || !valid {
		return DebtAging{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) GetReceivableAging(ctx context.Context, userID, organizationID, date string) (DebtAging, error) {
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
				SELECT r.due_date, r.amount - collections.collected AS amount
				FROM biznes.receivables r CROSS JOIN LATERAL (
					SELECT COALESCE(SUM(a.amount), 0) AS collected FROM biznes.receivable_allocations a
					WHERE a.organization_id = r.organization_id AND a.receivable_id = r.id
						AND NOT EXISTS (SELECT 1 FROM biznes.transaction_reversals v
							WHERE v.organization_id = a.organization_id AND v.transaction_id = a.transaction_id)
				) collections WHERE r.organization_id = m.organization_id
			) owed
		) totals WHERE m.organization_id = $1 AND m.user_id = $2`, organizationID, userID, asOf), asOf)
}
