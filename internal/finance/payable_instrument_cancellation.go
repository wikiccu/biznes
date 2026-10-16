package finance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wikiccu/biznes/internal/organization"
)

type InstrumentCancellation struct {
	ID, OrganizationID, InstrumentID, IdempotencyKey, Reason, CreatedBy string
	CreatedAt                                                           time.Time
}

type InstrumentCancellationInput = ReversalInput

var ErrInstrumentCancellationConflict = errors.New("instrument cancellation conflicts with existing record")

func scanInstrumentCancellation(row pgx.Row) (InstrumentCancellation, error) {
	var item InstrumentCancellation
	err := row.Scan(&item.ID, &item.OrganizationID, &item.InstrumentID, &item.IdempotencyKey,
		&item.Reason, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) CancelPayableInstrument(ctx context.Context, userID, organizationID, instrumentID string, input InstrumentCancellationInput) (InstrumentCancellation, bool, error) {
	input, err := validateReversal(input)
	if err != nil {
		return InstrumentCancellation{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return InstrumentCancellation{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return InstrumentCancellation{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return InstrumentCancellation{}, false, organization.ErrForbidden
	}
	item, err := scanInstrumentCancellation(tx.QueryRow(ctx, `INSERT INTO biznes.payable_instrument_cancellations
		(organization_id, instrument_id, idempotency_key, reason, created_by)
		SELECT organization_id, id, $3, $4, $5 FROM biznes.payable_instruments WHERE organization_id = $1 AND id = $2
		ON CONFLICT DO NOTHING
		RETURNING id, organization_id, instrument_id, idempotency_key, reason, created_by, created_at`,
		organizationID, instrumentID, input.IdempotencyKey, input.Reason, userID))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// Read the committed winner after either unique constraint serialized concurrent requests.
		item, err = scanInstrumentCancellation(tx.QueryRow(ctx, `SELECT id, organization_id, instrument_id, idempotency_key, reason, created_by, created_at
			FROM biznes.payable_instrument_cancellations WHERE organization_id = $1 AND (idempotency_key = $3 OR instrument_id = $2)
			ORDER BY (idempotency_key = $3) DESC LIMIT 1`, organizationID, instrumentID, input.IdempotencyKey))
		if errors.Is(err, pgx.ErrNoRows) {
			return InstrumentCancellation{}, false, organization.ErrNotFound
		}
		if err == nil && (item.InstrumentID != instrumentID || item.IdempotencyKey != input.IdempotencyKey || item.Reason != input.Reason) {
			return InstrumentCancellation{}, false, ErrInstrumentCancellationConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return InstrumentCancellation{}, false, organization.ErrNotFound
		}
		return InstrumentCancellation{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return InstrumentCancellation{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetPayableInstrumentCancellation(ctx context.Context, userID, organizationID, instrumentID string) (InstrumentCancellation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanInstrumentCancellation(s.pool.QueryRow(ctx, `SELECT r.id, r.organization_id, r.instrument_id, r.idempotency_key, r.reason, r.created_by, r.created_at
		FROM biznes.payable_instrument_cancellations r JOIN biznes.memberships m ON m.organization_id = r.organization_id
		WHERE r.organization_id = $1 AND r.instrument_id = $2 AND m.user_id = $3`, organizationID, instrumentID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return InstrumentCancellation{}, organization.ErrNotFound
	}
	if err != nil {
		return InstrumentCancellation{}, ErrUnavailable
	}
	return item, nil
}
