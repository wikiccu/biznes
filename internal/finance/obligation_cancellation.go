package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type ObligationCancellation struct {
	ID, OrganizationID, ObligationID, IdempotencyKey, Reason, CreatedBy string
	CreatedAt                                                           time.Time
}

type ObligationCancellationInput = ReversalInput

var ErrObligationCancellationConflict = errors.New("obligation cancellation conflicts with existing record")

func scanObligationCancellation(row pgx.Row) (ObligationCancellation, error) {
	var item ObligationCancellation
	err := row.Scan(&item.ID, &item.OrganizationID, &item.ObligationID, &item.IdempotencyKey,
		&item.Reason, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) CancelObligation(ctx context.Context, userID, organizationID, obligationID string, input ObligationCancellationInput) (ObligationCancellation, bool, error) {
	input, err := validateReversal(input)
	if err != nil {
		return ObligationCancellation{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ObligationCancellation{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return ObligationCancellation{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return ObligationCancellation{}, false, organization.ErrForbidden
	}
	item, err := scanObligationCancellation(tx.QueryRow(ctx, `INSERT INTO biznes.financial_obligation_cancellations
		(organization_id, obligation_id, idempotency_key, reason, created_by)
		SELECT organization_id, id, $3, $4, $5 FROM biznes.financial_obligations WHERE organization_id = $1 AND id = $2
		ON CONFLICT DO NOTHING
		RETURNING id, organization_id, obligation_id, idempotency_key, reason, created_by, created_at`,
		organizationID, obligationID, input.IdempotencyKey, input.Reason, userID))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// Read the committed winner after either unique constraint serialized concurrent requests.
		item, err = scanObligationCancellation(tx.QueryRow(ctx, `SELECT id, organization_id, obligation_id,
			idempotency_key, reason, created_by, created_at FROM biznes.financial_obligation_cancellations
			WHERE organization_id = $1 AND (idempotency_key = $3 OR obligation_id = $2)
			ORDER BY (idempotency_key = $3) DESC LIMIT 1`, organizationID, obligationID, input.IdempotencyKey))
		if errors.Is(err, pgx.ErrNoRows) {
			return ObligationCancellation{}, false, organization.ErrNotFound
		}
		if err == nil && (item.ObligationID != obligationID || item.IdempotencyKey != input.IdempotencyKey || item.Reason != input.Reason) {
			return ObligationCancellation{}, false, ErrObligationCancellationConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ObligationCancellation{}, false, organization.ErrNotFound
		}
		return ObligationCancellation{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return ObligationCancellation{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetObligationCancellation(ctx context.Context, userID, organizationID, obligationID string) (ObligationCancellation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanObligationCancellation(s.pool.QueryRow(ctx, `SELECT c.id, c.organization_id, c.obligation_id,
		c.idempotency_key, c.reason, c.created_by, c.created_at
		FROM biznes.financial_obligation_cancellations c JOIN biznes.memberships m ON m.organization_id = c.organization_id
		WHERE c.organization_id = $1 AND c.obligation_id = $2 AND m.user_id = $3`, organizationID, obligationID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ObligationCancellation{}, organization.ErrNotFound
	}
	if err != nil {
		return ObligationCancellation{}, ErrUnavailable
	}
	return item, nil
}
